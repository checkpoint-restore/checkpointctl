// SPDX-License-Identifier: Apache-2.0
//go:build linux

package cmd

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

type compressionHarness struct {
	directory string
	cli       string
	criu      string
}

func newCompressionHarness(t *testing.T) *compressionHarness {
	t.Helper()

	criu := os.Getenv("CRIU_TEST_BINARY")
	if criu == "" {
		t.Skip("set CRIU_TEST_BINARY to test CRIU-generated checkpoints")
	}
	if os.Geteuid() != 0 {
		t.Fatal("CRIU compression integration requires root")
	}
	if out, err := exec.Command(criu, "--no-default-config", "check", "--feature", "compress").CombinedOutput(); err != nil {
		t.Fatalf("CRIU requires LZ4 compression support: %v\n%s", err, out)
	}

	directory, err := os.MkdirTemp("/tmp", "checkpointctl-compression-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}

	h := &compressionHarness{
		directory: directory,
		cli:       filepath.Join(directory, "checkpointctl"),
		criu:      criu,
	}
	// Disable VCS metadata to avoid Git ownership errors when root builds a
	// checkout owned by another user.
	if out, err := exec.Command("go", "build", "-buildvcs=false", "-o", h.cli, "..").CombinedOutput(); err != nil {
		t.Fatalf("build checkpointctl: %v\n%s", err, out)
	}
	return h
}

func (h *compressionHarness) workload(t *testing.T, name string, args, environment []string) (*exec.Cmd, []byte) {
	t.Helper()

	executable := filepath.Join(h.directory, name)
	if out, err := exec.Command("cc", "-Wall", "-Wextra", "-Werror", "-o", executable, "../test/compression/"+name+".c").CombinedOutput(); err != nil {
		t.Fatalf("build workload: %v\n%s", err, out)
	}

	logPath := filepath.Join(h.directory, name+".stderr")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	process := exec.Command(executable, args...)
	process.Stderr = log
	process.Env = append([]string{
		"PATH=/usr/bin:/bin",
		"CHECKPOINTCTL_TEST=CRIU_COMPRESSION_MARKER",
		"CHECKPOINTCTL_EMPTY=",
	}, environment...)
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	ready, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = process.Process.Signal(syscall.SIGTERM)
		exited := make(chan struct{})
		go func() {
			_ = process.Wait()
			close(exited)
		}()
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			_ = syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
			<-exited
		}

		// A failed parent can exit before reaping its children.
		_ = syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		if t.Failed() {
			if data, err := os.ReadFile(logPath); err == nil && len(data) != 0 {
				t.Logf("workload stderr:\n%s", data)
			}
		}
	})

	// Workloads close stdout only after completing setup, before any checkpoint.
	type readiness struct {
		data []byte
		err  error
	}
	results := make(chan readiness, 1)
	go func() {
		data, err := io.ReadAll(ready)
		results <- readiness{data, err}
	}()

	select {
	case result := <-results:
		if result.err != nil {
			t.Fatal(result.err)
		}
		return process, result.data
	case <-time.After(10 * time.Second):
		_ = syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		t.Fatal("workload readiness timed out")
		return nil, nil
	}
}

func (h *compressionHarness) run(t *testing.T, args ...string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, h.cli, args...)
	command.Dir = h.directory
	command.Env = []string{"PATH=/usr/bin:/bin", "HOME=/tmp"}
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("unprivileged command %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func (h *compressionHarness) dump(t *testing.T, pid int, name string, block int) (string, string) {
	t.Helper()

	images := filepath.Join(h.directory, name)
	if err := os.Mkdir(images, 0o700); err != nil {
		t.Fatal(err)
	}

	args := []string{
		"--no-default-config", "dump", "--shell-job", "--leave-running",
		"--libdir", h.directory,
		"-D", images,
		"-o", "dump.log",
		"-v4", "-t", strconv.Itoa(pid),
	}
	if block != 0 {
		args = append(args, "--compress-block="+strconv.Itoa(block))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, h.criu, args...).CombinedOutput(); err != nil {
		log, _ := os.ReadFile(filepath.Join(images, "dump.log"))
		t.Fatalf("create CRIU checkpoint: %v\n%s\n%s", err, output, log)
	}
	return images, h.archive(t, images)
}

func (h *compressionHarness) archive(t *testing.T, images string) string {
	t.Helper()

	files := map[string][]byte{}
	entries, err := os.ReadDir(images)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(images, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		name := "checkpoint/" + entry.Name()
		if entry.Name() == "stats-dump" {
			name = "stats-dump"
		}
		files[name] = data
	}

	files["config.dump"] = []byte(`{"id":"compression-integration","name":"compression-integration"}`)
	files["spec.dump"] = []byte(`{"annotations":{"io.container.manager":"libpod"}}`)
	path := images + ".tar"
	writeCheckpointArchive(t, path, files)
	return path
}
