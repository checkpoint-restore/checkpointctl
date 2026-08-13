// SPDX-License-Identifier: Apache-2.0

//go:build unix

package metadata

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadJSONFileRejectsFIFO(t *testing.T) {
	tmpDir := t.TempDir()
	fifoPath := filepath.Join(tmpDir, SpecDumpFile)
	if err := unix.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("failed to create FIFO: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		var specDump map[string]interface{}
		_, err := ReadJSONFile(&specDump, tmpDir, SpecDumpFile)
		result <- err
	}()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected an error for a FIFO, got nil")
		}
		if !errors.Is(err, errNotRegularFile) {
			t.Fatalf("expected a non-regular file error, got %v", err)
		}
	case <-time.After(time.Second):
		// Unblock a vulnerable implementation before failing the test.
		if writer, err := os.OpenFile(fifoPath, os.O_WRONLY|unix.O_NONBLOCK, 0); err == nil {
			_ = writer.Close()
		}
		t.Fatal("ReadJSONFile blocked while opening a FIFO")
	}
}

func TestOpenRegularFileRejectsFIFOWithoutBlocking(t *testing.T) {
	tmpDir := t.TempDir()
	fifoPath := filepath.Join(tmpDir, SpecDumpFile)
	if err := unix.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("failed to create FIFO: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		f, err := openRegularFile(fifoPath)
		if f != nil {
			_ = f.Close()
		}
		result <- err
	}()

	select {
	case err := <-result:
		if !errors.Is(err, errNotRegularFile) {
			t.Fatalf("expected a non-regular file error, got %v", err)
		}
	case <-time.After(time.Second):
		// Unblock an implementation that opens the FIFO without O_NONBLOCK.
		if writer, err := os.OpenFile(fifoPath, os.O_WRONLY|unix.O_NONBLOCK, 0); err == nil {
			_ = writer.Close()
		}
		t.Fatal("openRegularFile blocked while opening a FIFO")
	}
}

func TestReadJSONFileRejectsSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "target.json")
	if err := os.WriteFile(targetPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to create target file: %v", err)
	}

	symlinkPath := filepath.Join(tmpDir, SpecDumpFile)
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatalf("failed to create symbolic link: %v", err)
	}

	var specDump map[string]interface{}
	_, err := ReadJSONFile(&specDump, tmpDir, SpecDumpFile)
	if !errors.Is(err, errNotRegularFile) {
		t.Fatalf("expected a non-regular file error, got %v", err)
	}
}

func TestOpenRegularFileDoesNotFollowSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "target.json")
	if err := os.WriteFile(targetPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to create target file: %v", err)
	}

	symlinkPath := filepath.Join(tmpDir, SpecDumpFile)
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatalf("failed to create symbolic link: %v", err)
	}

	f, err := openRegularFile(symlinkPath)
	if f != nil {
		_ = f.Close()
	}
	if err == nil {
		t.Fatal("expected an error for a symbolic link, got nil")
	}
}

func TestOpenRegularFileRejectsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := openRegularFile(tmpDir)
	if f != nil {
		_ = f.Close()
	}
	if !errors.Is(err, errNotRegularFile) {
		t.Fatalf("expected a non-regular file error, got %v", err)
	}
}

func TestOpenRegularFileRejectsUnixSocket(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "metadata.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to create Unix socket: %v", err)
	}
	defer listener.Close()

	f, err := openRegularFile(socketPath)
	if f != nil {
		_ = f.Close()
	}
	if !errors.Is(err, errNotRegularFile) {
		t.Fatalf("expected a non-regular file error, got %v", err)
	}
}
