// SPDX-License-Identifier: Apache-2.0
//go:build linux

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/checkpoint-restore/checkpointctl/internal"
	"golang.org/x/sys/unix"
)

type compressionState struct {
	Phase        string `json:"phase"`
	Root         uint32 `json:"root"`
	Child        uint32 `json:"child"`
	ChildAddress uint64 `json:"child_address"`
	ChildSize    int    `json:"child_size"`
}

func loadCompressionState(t *testing.T, directory string) compressionState {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	var state compressionState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func changeCompressionState(t *testing.T, process *os.Process, directory string) {
	t.Helper()

	// Watch before signalling. Atomic manifest rename acknowledges that all
	// arguments and environment reached the new state.
	watch, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(watch)
	if _, err := unix.InotifyAddWatch(watch, directory, unix.IN_MOVED_TO); err != nil {
		t.Fatal(err)
	}

	if err := process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}

	poll := []unix.PollFd{{Fd: int32(watch), Events: unix.POLLIN}}
	deadline := time.Now().Add(5 * time.Second)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatal("state transition timed out")
		}
		n, err := unix.Poll(poll, int(remaining.Milliseconds())+1)
		if err == unix.EINTR {
			continue
		}
		if err != nil || n != 1 {
			t.Fatalf("state transition timed out: n=%d, err=%v", n, err)
		}
		break
	}
}

func TestCRIUCompressedStateChanges(t *testing.T) {
	h := newCompressionHarness(t)
	process, ready := h.workload(t, "stateful",
		[]string{h.directory, "before"},
		[]string{"CHECKPOINTCTL_STATE=before"},
	)
	if string(ready) != "ready\n" {
		t.Fatalf("workload setup failed: %q", ready)
	}

	before := loadCompressionState(t, h.directory)
	_, beforeArchive := h.dump(t, process.Process.Pid, "before", 256*1024)
	assertCompressionStateView(t, h, beforeArchive, before, "before")
	assertChildMemory(t, h, beforeArchive, before)

	changeCompressionState(t, process.Process, h.directory)

	after := loadCompressionState(t, h.directory)
	if after.Phase != "after" || after.Root != before.Root || after.Child != before.Child {
		t.Fatalf("unexpected transition: before=%+v after=%+v", before, after)
	}
	_, afterArchive := h.dump(t, process.Process.Pid, "after", 256*1024)
	assertCompressionStateView(t, h, afterArchive, after, "after!")
	assertChildMemory(t, h, afterArchive, after)

	args := []string{
		"diff", beforeArchive, afterArchive,
		"--ps-tree-cmd", "--ps-tree-env", "--format=json",
	}
	output := h.run(t, args...)
	var result DiffResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}

	changes := result.ProcessChanges
	if changes == nil {
		t.Fatal("missing process changes")
	}
	if len(changes.Added) != 0 || len(changes.Removed) != 0 {
		t.Fatalf("processes unexpectedly added or removed: %+v", changes)
	}

	if len(changes.Modified) != 1 {
		t.Fatalf("modified processes: got %+v, want root PID %d", changes.Modified, before.Root)
	}
	if changes.Modified[0].PID != int(before.Root) {
		t.Fatalf("modified PID: got %d, want %d", changes.Modified[0].PID, before.Root)
	}

	if len(changes.Unchanged) != 1 {
		t.Fatalf("unchanged processes: got %+v, want child PID %d", changes.Unchanged, before.Child)
	}
	if changes.Unchanged[0].PID != int(before.Child) {
		t.Fatalf("unchanged PID: got %d, want %d", changes.Unchanged[0].PID, before.Child)
	}

	modified := changes.Modified[0]
	if modified.EnvVars["CHECKPOINTCTL_STATE"] != "after!" {
		t.Fatalf("changed process data not decoded: %+v", modified)
	}
	if !strings.HasSuffix(strings.TrimSpace(modified.Cmdline), "after!") {
		t.Fatalf("changed process data not decoded: %+v", modified)
	}

	// Check each option separately so one working comparison cannot hide a
	// missing argument or environment comparison.
	for _, flag := range []string{"--ps-tree-cmd", "--ps-tree-env", ""} {
		arguments := []string{"diff", beforeArchive, afterArchive, "--format=json"}
		if flag != "" {
			arguments = append(arguments, flag)
		}
		comparison := h.run(t, arguments...)
		var filtered DiffResult
		if err := json.Unmarshal([]byte(comparison), &filtered); err != nil {
			t.Fatal(err)
		}

		wantModified := 1
		if flag == "" {
			wantModified = 0
		}
		filteredChanges := filtered.ProcessChanges
		if filteredChanges == nil {
			t.Fatalf("missing process changes for %q: %s", flag, comparison)
		}
		if len(filteredChanges.Modified) != wantModified || len(filteredChanges.Unchanged) != 2-wantModified {
			t.Fatalf("%q did not select the expected process comparison: %s", flag, comparison)
		}
	}
}

func assertCompressionStateView(t *testing.T, h *compressionHarness, archive string, state compressionState, value string) {
	t.Helper()

	output := h.run(t, "inspect", archive, "--all", "--format=json")
	var views []internal.DisplayNode
	if err := json.Unmarshal([]byte(output), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].ProcessTree == nil {
		t.Fatalf("missing process tree: %s", output)
	}

	tree := views[0].ProcessTree
	if tree.PID != state.Root {
		t.Fatalf("root PID: got %d, want %d", tree.PID, state.Root)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("children: got %+v, want PID %d", tree.Children, state.Child)
	}
	if tree.Children[0].PID != state.Child {
		t.Fatalf("child PID: got %d, want %d", tree.Children[0].PID, state.Child)
	}

	if got := tree.EnvVars["CHECKPOINTCTL_STATE"]; got != value {
		t.Fatalf("state environment: got %q, want %q", got, value)
	}
	if !strings.HasSuffix(strings.TrimSpace(tree.Cmdline), value) {
		t.Fatalf("command line does not end with %q: %q", value, tree.Cmdline)
	}

	filtered := h.run(t, "inspect", archive,
		"--pid", strconv.Itoa(int(state.Child)),
		"--ps-tree-cmd", "--ps-tree-env", "--format=json",
	)
	var subset []internal.DisplayNode
	if err := json.Unmarshal([]byte(filtered), &subset); err != nil {
		t.Fatal(err)
	}
	if len(subset) != 1 || subset[0].ProcessTree == nil {
		t.Fatalf("PID filter did not select child: %s", filtered)
	}
	child := subset[0].ProcessTree
	if child.PID != state.Child || len(child.Children) != 0 {
		t.Fatalf("PID filter did not select child: %s", filtered)
	}
}

func assertChildMemory(t *testing.T, h *compressionHarness, archive string, state compressionState) {
	t.Helper()

	expected := bytes.Repeat([]byte{'K'}, state.ChildSize)
	page := state.ChildSize / 2
	copy(expected[page-8:], "CHILD_PRIVATE_MARKER")
	copy(expected[len(expected)-16:], "END_CHILD_MEMORY")

	file := filepath.Join(h.directory, "child-"+state.Phase)
	if err := os.WriteFile(file, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0o666); err != nil {
		t.Fatal(err)
	}
	h.run(t, "memparse", archive, "--pid", strconv.Itoa(int(state.Child)), "--output", file)
	verifyMemoryDump(t, file, state.ChildAddress, expected)

	markerAddress := state.ChildAddress + uint64(page-8)
	for _, search := range []string{
		"--search=CHILD_PRIVATE_MARKER",
		"--search-regex=CHILD_PRIVATE_M[A]+RKER",
	} {
		output := h.run(t, "memparse", archive, "--pid", strconv.Itoa(int(state.Child)), search, "--context=4")
		assertSearchRow(t, output, markerAddress, "KKKKCHILD_PRIVATE_MARKERKKKK")
	}

	compact := h.run(t, "memparse", archive, "--pid", strconv.Itoa(int(state.Child)))
	verifyCompactMapping(t, compact, state.ChildAddress, expected)
}
