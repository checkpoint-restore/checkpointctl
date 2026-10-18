// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/checkpoint-restore/checkpointctl/internal"
	metadata "github.com/checkpoint-restore/checkpointctl/lib"
)

// compareSockets [empty inputs]
func TestCompareSocketsEmptyInputs(t *testing.T) {
	result := compareSockets([]internal.SkNode{}, []internal.SkNode{})

	if result == nil {
		t.Fatal("Expected non-nil result for empty inputs")
	}

	if len(result.Added) != 0 {
		t.Errorf("Expected 0 added sockets, got %d", len(result.Added))
	}

	if len(result.Removed) != 0 {
		t.Errorf("Expected 0 removed sockets, got %d", len(result.Removed))
	}

	if len(result.Unchanged) != 0 {
		t.Errorf("Expected 0 unchanged sockets, got %d", len(result.Unchanged))
	}
}

func TestCompareSocketsNilInputs(t *testing.T) {
	result := compareSockets(nil, nil)

	if result == nil {
		t.Fatal("Expected non-nil result for nil inputs")
	}

	if len(result.Added) != 0 {
		t.Errorf("Expected 0 added sockets for nil inputs, got %d", len(result.Added))
	}

	if len(result.Removed) != 0 {
		t.Errorf("Expected 0 removed sockets for nil inputs, got %d", len(result.Removed))
	}

	if len(result.Unchanged) != 0 {
		t.Errorf("Expected 0 unchanged sockets for nil inputs, got %d", len(result.Unchanged))
	}
}

// added sockets
func TestCompareSocketsAddedSockets(t *testing.T) {
	socketA := internal.SkNode{
		PID: 1234,
		OpenSockets: []internal.SocketNode{
			{
				Protocol: "TCP",
				Data: internal.SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8080,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	socketB := internal.SkNode{
		PID: 1234,
		OpenSockets: []internal.SocketNode{
			{
				Protocol: "TCP",
				Data: internal.SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8080,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
			{
				Protocol: "TCP",
				Data: internal.SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8081,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	result := compareSockets([]internal.SkNode{socketA}, []internal.SkNode{socketB})

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if len(result.Added) != 1 {
		t.Errorf("Expected 1 added socket, got %d", len(result.Added))
	}

	if len(result.Removed) != 0 {
		t.Errorf("Expected 0 removed sockets, got %d", len(result.Removed))
	}

	if len(result.Unchanged) != 1 {
		t.Errorf("Expected 1 unchanged socket, got %d", len(result.Unchanged))
	}

	// verify added socket is the correct one (port 8081)
	if result.Added[0].SrcPort != 8081 {
		t.Errorf("Expected added socket to have port 8081, got %d", result.Added[0].SrcPort)
	}
}

// removed sockets
func TestCompareSocketsRemovedSockets(t *testing.T) {
	socketA := internal.SkNode{
		PID: 1234,
		OpenSockets: []internal.SocketNode{
			{
				Protocol: "TCP",
				Data: internal.SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8080,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
			{
				Protocol: "TCP",
				Data: internal.SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8081,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	socketB := internal.SkNode{
		PID: 1234,
		OpenSockets: []internal.SocketNode{
			{
				Protocol: "TCP",
				Data: internal.SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8081,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	result := compareSockets([]internal.SkNode{socketA}, []internal.SkNode{socketB})

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if len(result.Added) != 0 {
		t.Errorf("Expected 0 added sockets, got %d", len(result.Added))
	}

	if len(result.Removed) != 1 {
		t.Errorf("Expected 1 removed socket, got %d", len(result.Removed))
	}

	if len(result.Unchanged) != 1 {
		t.Errorf("Expected 1 unchanged socket, got %d", len(result.Unchanged))
	}

	// verify removed socket is the correct one (port 8080)
	if result.Removed[0].SrcPort != 8080 {
		t.Errorf("Expected removed socket to have port 8080, got %d", result.Removed[0].SrcPort)
	}
}

// comparing sockets from different processes
func TestCompareSocketsMultiplePIDs(t *testing.T) {
	socketPID1 := internal.SkNode{
		PID: 1,
		OpenSockets: []internal.SocketNode{
			{
				Protocol: "TCP",
				Data: internal.SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 80,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	socketPID2 := internal.SkNode{
		PID: 2,
		OpenSockets: []internal.SocketNode{
			{
				Protocol: "TCP",
				Data: internal.SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 443,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	result := compareSockets([]internal.SkNode{socketPID1, socketPID2}, []internal.SkNode{socketPID1})

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if len(result.Removed) != 1 {
		t.Errorf("Expected 1 removed socket (PID 2's socket), got %d", len(result.Removed))
	}

	if result.Removed[0].PID != 2 {
		t.Errorf("Expected removed socket to be from PID 2, got PID %d", result.Removed[0].PID)
	}

	if result.Removed[0].SrcPort != 443 {
		t.Errorf("Expected removed socket to have port 443, got %d", result.Removed[0].SrcPort)
	}
}

// summary generation with socket changes | for test generateSummary()
func TestGenerateSummaryWithSocketChanges(t *testing.T) {
	result := &DiffResult{
		ContainerName: "test-container",
		SocketChanges: &SocketDiff{
			Added: []SocketInfo{
				{PID: 1, Protocol: "TCP", SrcPort: 8081},
				{PID: 1, Protocol: "TCP", SrcPort: 8082},
			},
			Removed: []SocketInfo{
				{PID: 1, Protocol: "TCP", SrcPort: 8080},
			},
			Unchanged: nil,
		},
	}

	summary := generateSummary(result)

	if summary == "" {
		t.Fatal("Expected non-empty summary")
	}

	// Check that socket changes are mentioned
	if expected := "Sockets: +2 -1"; summary != expected &&
		(summary[len(summary)-9:] != "+2 -1" && summary[len(summary)-9:] != "2 -1") {
		t.Logf("Summary: %s", summary)
		//
		if !contains(summary, "Sockets:") {
			t.Errorf("Expected summary to contain 'Sockets:', got '%s'", summary)
		}
	}
}

// tests sockets with missing/empty field values
func TestCompareSocketsWithEmptyFields(t *testing.T) {
	socketA := internal.SkNode{
		PID: 1234,
		OpenSockets: []internal.SocketNode{
			{
				Protocol: "UNIX",
				Data: internal.SkData{
					Type:    "UNIX",
					Address: "", // Empty address
				},
			},
		},
	}

	socketB := internal.SkNode{
		PID: 1234,
		OpenSockets: []internal.SocketNode{
			{
				Protocol: "UNIX",
				Data: internal.SkData{
					Type:    "UNIX",
					Address: "", //
				},
			},
		},
	}

	result := compareSockets([]internal.SkNode{socketA}, []internal.SkNode{socketB})

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Should be treated as identical (both have empty address)
	if len(result.Added) != 0 {
		t.Errorf("Expected 0 added sockets, got %d", len(result.Added))
	}

	if len(result.Removed) != 0 {
		t.Errorf("Expected 0 removed sockets, got %d", len(result.Removed))
	}

	if len(result.Unchanged) != 1 {
		t.Errorf("Expected 1 unchanged socket, got %d", len(result.Unchanged))
	}
}

// helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// nil inputs
func TestCompareProcessTreesNilInputs(t *testing.T) {
	result := compareProcessTrees(nil, nil)

	if result == nil {
		t.Fatal("Expected non-nil result for nil inputs")
	}
	if len(result.Added) != 0 || len(result.Removed) != 0 ||
		len(result.Modified) != 0 || len(result.Unchanged) != 0 {
		t.Errorf("Expected all diff buckets empty, got %+v", result)
	}
}

// identical trees
func TestCompareProcessTreesIdentical(t *testing.T) {
	tree := &internal.PsNode{
		PID: 1, Comm: "init",
		Children: []internal.PsNode{
			{PID: 2, Comm: "sh"},
			{PID: 3, Comm: "bash"},
		},
	}

	result := compareProcessTrees(tree, tree)

	if len(result.Added) != 0 {
		t.Errorf("Expected 0 added, got %d", len(result.Added))
	}
	if len(result.Removed) != 0 {
		t.Errorf("Expected 0 removed, got %d", len(result.Removed))
	}
	if len(result.Modified) != 0 {
		t.Errorf("Expected 0 modified, got %d", len(result.Modified))
	}
	if len(result.Unchanged) != 3 {
		t.Errorf("Expected 3 unchanged, got %d", len(result.Unchanged))
	}
}

// added and removed PIDs
func TestCompareProcessTreesAddedRemoved(t *testing.T) {
	treeA := &internal.PsNode{
		PID: 1, Comm: "init",
		Children: []internal.PsNode{
			{PID: 2, Comm: "sh"},
			{PID: 3, Comm: "bash"},
		},
	}
	treeB := &internal.PsNode{
		PID: 1, Comm: "init",
		Children: []internal.PsNode{
			{PID: 2, Comm: "sh"},
			{PID: 4, Comm: "python"},
		},
	}

	result := compareProcessTrees(treeA, treeB)

	if len(result.Added) != 1 || result.Added[0].PID != 4 {
		t.Errorf("Expected 1 added PID=4, got %+v", result.Added)
	}
	if len(result.Removed) != 1 || result.Removed[0].PID != 3 {
		t.Errorf("Expected 1 removed PID=3, got %+v", result.Removed)
	}
	if len(result.Unchanged) != 2 {
		t.Errorf("Expected 2 unchanged (PIDs 1, 2), got %d", len(result.Unchanged))
	}
}

// cmdline change with --ps-tree-cmd
func TestCompareProcessTreesModifiedCmdline(t *testing.T) {
	prev := internal.PsTreeCmd
	internal.PsTreeCmd = true
	defer func() { internal.PsTreeCmd = prev }()

	treeA := &internal.PsNode{PID: 1, Comm: "server", Cmdline: "server --port=80"}
	treeB := &internal.PsNode{PID: 1, Comm: "server", Cmdline: "server --port=443"}

	result := compareProcessTrees(treeA, treeB)

	if len(result.Modified) != 1 || result.Modified[0].PID != 1 {
		t.Errorf("Expected 1 modified PID=1, got %+v", result.Modified)
	}
	if result.Modified[0].Cmdline != "server --port=443" {
		t.Errorf("Expected modified cmdline to reflect B, got %q", result.Modified[0].Cmdline)
	}
	if len(result.Unchanged) != 0 {
		t.Errorf("Expected 0 unchanged, got %d", len(result.Unchanged))
	}
}

// cmdline change without --ps-tree-cmd
func TestCompareProcessTreesCmdlineIgnoredByDefault(t *testing.T) {
	prev := internal.PsTreeCmd
	internal.PsTreeCmd = false
	defer func() { internal.PsTreeCmd = prev }()

	treeA := &internal.PsNode{PID: 1, Comm: "server", Cmdline: "server --port=80"}
	treeB := &internal.PsNode{PID: 1, Comm: "server", Cmdline: "server --port=443"}

	result := compareProcessTrees(treeA, treeB)

	if len(result.Modified) != 0 {
		t.Errorf("Expected 0 modified without --ps-tree-cmd, got %d", len(result.Modified))
	}
	if len(result.Unchanged) != 1 {
		t.Errorf("Expected 1 unchanged, got %d", len(result.Unchanged))
	}
}

func TestCompareProcessTreesEnvVars(t *testing.T) {
	prevEnv, prevCmd := internal.PsTreeEnv, internal.PsTreeCmd
	t.Cleanup(func() {
		internal.PsTreeEnv, internal.PsTreeCmd = prevEnv, prevCmd
	})
	internal.PsTreeCmd = false

	tests := []struct {
		name     string
		before   map[string]string
		after    map[string]string
		modified bool
	}{
		{"changed", map[string]string{"VAR": "before"}, map[string]string{"VAR": "after"}, true},
		{"added", nil, map[string]string{"VAR": "value"}, true},
		{"removed", map[string]string{"VAR": "value"}, nil, true},
		{"added empty value", nil, map[string]string{"VAR": ""}, true},
		{"removed empty value", map[string]string{"VAR": ""}, nil, true},
		{"unchanged", map[string]string{"A": "", "B": "value"}, map[string]string{"B": "value", "A": ""}, false},
		{"nil and empty", nil, map[string]string{}, false},
	}
	for _, tt := range tests {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/env=%t", tt.name, enabled), func(t *testing.T) {
				internal.PsTreeEnv = enabled
				before := &internal.PsNode{PID: 1, Comm: "server", EnvVars: tt.before}
				after := &internal.PsNode{PID: 1, Comm: "server", EnvVars: tt.after}
				result := compareProcessTrees(before, after)

				wantModified := 0
				if enabled && tt.modified {
					wantModified = 1
				}
				if len(result.Modified) != wantModified || len(result.Unchanged) != 1-wantModified {
					t.Errorf("Expected %d modified and %d unchanged, got %+v", wantModified, 1-wantModified, result)
				}
				if len(result.Added) != 0 || len(result.Removed) != 0 {
					t.Errorf("Expected no added or removed processes, got %+v", result)
				}
			})
		}
	}
}

func TestRenderDiffEnvVars(t *testing.T) {
	prevEnv, prevCmd, prevUnchanged := internal.PsTreeEnv, internal.PsTreeCmd, internal.ShowUnchanged
	t.Cleanup(func() {
		internal.PsTreeEnv, internal.PsTreeCmd, internal.ShowUnchanged = prevEnv, prevCmd, prevUnchanged
	})
	internal.PsTreeCmd = false

	before := CheckpointMetadata{ProcessTree: &internal.PsNode{
		PID: 1, Comm: "init", EnvVars: map[string]string{"TEST_UNCHANGED": "kept"},
		Children: []internal.PsNode{
			{PID: 2, Comm: "server", EnvVars: map[string]string{"TEST_CHANGED": "before", "TEST_EMPTY": ""}},
			{PID: 3, Comm: "removed", EnvVars: map[string]string{"TEST_REMOVED": "before"}},
		},
	}}
	after := CheckpointMetadata{ProcessTree: &internal.PsNode{
		PID: 1, Comm: "init", EnvVars: map[string]string{"TEST_UNCHANGED": "kept"},
		Children: []internal.PsNode{
			{PID: 2, Comm: "server", EnvVars: map[string]string{"TEST_EMPTY": "", "TEST_CHANGED": "after"}},
			{PID: 4, Comm: "added", EnvVars: map[string]string{"TEST_ADDED": "after"}},
		},
	}}

	for _, tt := range []struct {
		name          string
		showUnchanged bool
		json          bool
	}{
		{"tree", false, false},
		{"tree with unchanged", true, false},
		{"json", false, true},
	} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/env=%t", tt.name, enabled), func(t *testing.T) {
				internal.PsTreeEnv, internal.ShowUnchanged = enabled, tt.showUnchanged
				result := computeDiff(before, after)
				out := captureDiffOutput(t, func() {
					if tt.json {
						if err := renderJSONDiff(result); err != nil {
							t.Fatal(err)
						}
					} else {
						renderTreeDiff(result)
					}
				})

				for key, value := range map[string]string{
					"TEST_ADDED":     "after",
					"TEST_CHANGED":   "after",
					"TEST_EMPTY":     "",
					"TEST_REMOVED":   "before",
					"TEST_UNCHANGED": "kept",
				} {
					want := enabled && (key != "TEST_UNCHANGED" || tt.showUnchanged || tt.json)
					entry := key + "=" + value
					if tt.json {
						entry = fmt.Sprintf("%q: %q", key, value)
					}
					if want && !strings.Contains(out, entry) {
						t.Errorf("Expected output to contain %q, got:\n%s", entry, out)
					} else if !want && strings.Contains(out, key) {
						t.Errorf("Unexpected environment variable %s in output:\n%s", key, out)
					}
				}
				if tt.json && strings.Contains(out, `"environment_variables"`) != enabled {
					t.Errorf("Environment field presence does not match --ps-tree-env=%t:\n%s", enabled, out)
				}
				if enabled && !tt.json && strings.Index(out, "TEST_CHANGED=") > strings.Index(out, "TEST_EMPTY=") {
					t.Errorf("Environment variables are not sorted:\n%s", out)
				}
			})
		}
	}
}

func captureDiffOutput(t *testing.T, render func()) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "diff-output")
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = file
	defer func() {
		os.Stdout = stdout
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	}()
	render()
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// flattening a nested tree visits every node
func TestFlattenProcessTreeNested(t *testing.T) {
	tree := &internal.PsNode{
		PID: 1, Comm: "init",
		Children: []internal.PsNode{
			{
				PID: 2, Comm: "sh",
				Children: []internal.PsNode{
					{PID: 5, Comm: "grandchild"},
				},
			},
			{PID: 3, Comm: "bash"},
		},
	}

	procs := flattenProcessTree(tree)

	if len(procs) != 4 {
		t.Fatalf("Expected 4 processes, got %d", len(procs))
	}
	seen := map[int]bool{}
	for _, p := range procs {
		seen[p.PID] = true
	}
	for _, pid := range []int{1, 2, 3, 5} {
		if !seen[pid] {
			t.Errorf("Expected PID %d in flattened list", pid)
		}
	}
}

// flattening a nil tree returns nil
func TestFlattenProcessTreeNil(t *testing.T) {
	if procs := flattenProcessTree(nil); procs != nil {
		t.Errorf("Expected nil for nil input, got %+v", procs)
	}
}

// buildProcessStatusMap maps each PID to its marker
func TestBuildProcessStatusMap(t *testing.T) {
	diff := &ProcessDiff{
		Added:     []ProcessInfo{{PID: 10}},
		Modified:  []ProcessInfo{{PID: 20}},
		Unchanged: []ProcessInfo{{PID: 30}, {PID: 31}},
		Removed:   []ProcessInfo{{PID: 40}},
	}

	status := buildProcessStatusMap(diff)

	cases := map[uint32]string{10: "+", 20: "~", 30: "=", 31: "="}
	for pid, want := range cases {
		if got := status[pid]; got != want {
			t.Errorf("status[%d] = %q, want %q", pid, got, want)
		}
	}
	if _, ok := status[40]; ok {
		t.Errorf("Removed PID 40 should not appear in status map")
	}
	if len(status) != 4 {
		t.Errorf("Expected 4 entries in status map, got %d", len(status))
	}
}

// renderAnnotatedProcessTree emits one line per PID with its marker
func TestRenderAnnotatedProcessTree(t *testing.T) {
	tree := &internal.PsNode{
		PID: 1, Comm: "init",
		Children: []internal.PsNode{
			{PID: 2, Comm: "kept"},
			{PID: 3, Comm: "new"},
		},
	}
	status := map[uint32]string{1: "=", 2: "=", 3: "+"}

	out := renderAnnotatedProcessTree(tree, status)

	for _, want := range []string{
		"Process tree",
		"= PID 1",
		"= PID 2",
		"+ PID 3",
		"kept",
		"new",
	} {
		if !contains(out, want) {
			t.Errorf("Expected output to contain %q, got:\n%s", want, out)
		}
	}
}

// renderAnnotatedProcessTree uses a blank marker for unknown PIDs
func TestRenderAnnotatedProcessTreeUnknownPID(t *testing.T) {
	tree := &internal.PsNode{PID: 99, Comm: "mystery"}
	out := renderAnnotatedProcessTree(tree, map[uint32]string{})

	if !contains(out, "  PID 99") {
		t.Errorf("Expected blank marker for unknown PID, got:\n%s", out)
	}
}

func createTestCheckpointArchive(t *testing.T, dir, filename string, containerID, containerName, runtime, imageName string, created time.Time) string {
	t.Helper()
	archivePath := filepath.Join(dir, filename)
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("creating test archive: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("closing test archive: %v", err)
		}
	}()

	tw := tar.NewWriter(f)
	defer func() {
		if err := tw.Close(); err != nil {
			t.Errorf("finishing test archive: %v", err)
		}
	}()

	// 1. checkpoint/ directory
	dirHdr := &tar.Header{
		Name:     "checkpoint/",
		Typeflag: tar.TypeDir,
		Mode:     0o700,
	}
	if err := tw.WriteHeader(dirHdr); err != nil {
		t.Fatalf("writing dir header: %v", err)
	}

	// 2. spec.dump
	specContent := `{"annotations":{"io.container.manager":"libpod"}}`
	specHdr := &tar.Header{
		Name: "spec.dump",
		Mode: 0o600,
		Size: int64(len(specContent)),
	}
	if err := tw.WriteHeader(specHdr); err != nil {
		t.Fatalf("writing spec header: %v", err)
	}
	if _, err := tw.Write([]byte(specContent)); err != nil {
		t.Fatalf("writing spec content: %v", err)
	}

	// 3. config.dump
	configObj := metadata.ContainerConfig{
		ID:              containerID,
		Name:            containerName,
		OCIRuntime:      runtime,
		CreatedTime:     created,
		RootfsImageName: imageName,
	}
	configBytes, err := json.Marshal(configObj)
	if err != nil {
		t.Fatalf("marshaling config: %v", err)
	}
	configHdr := &tar.Header{
		Name: "config.dump",
		Mode: 0o600,
		Size: int64(len(configBytes)),
	}
	if err := tw.WriteHeader(configHdr); err != nil {
		t.Fatalf("writing config header: %v", err)
	}
	if _, err := tw.Write(configBytes); err != nil {
		t.Fatalf("writing config content: %v", err)
	}

	return archivePath
}

// runDiffTestSubprocess isolates tests that could hang with stdout redirected.
// The parent can kill a blocked child without leaving its own stdout or globals
// changed. Return true in the parent so the caller skips the child-only work.
func runDiffTestSubprocess(t *testing.T) bool {
	t.Helper()
	const childEnv = "CHECKPOINTCTL_DIFF_TEST_CHILD"
	if os.Getenv(childEnv) == t.Name() {
		return false
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(t.Name(), "/")
	for i, part := range parts {
		parts[i] = "^" + regexp.QuoteMeta(part) + "$"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run="+strings.Join(parts, "/"))
	// Keep files from a killed child under the parent's cleanup directory.
	cmd.Env = append(os.Environ(), childEnv+"="+t.Name(), "TMPDIR="+t.TempDir())
	cmd.WaitDelay = time.Second
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%s timed out: %v\n%s", t.Name(), ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("%s failed in subprocess: %v\n%s", t.Name(), err, output)
	}
	return true
}

func TestGetTaskJSON_LargeOutput(t *testing.T) {
	testCreated := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	testCases := []struct {
		name      string
		imageSize int
	}{
		{
			name:      "75_KiB",
			imageSize: 75 * 1024,
		},
		{
			name:      "1500_KiB",
			imageSize: 1500 * 1024,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if runDiffTestSubprocess(t) {
				return
			}
			tmpDir := t.TempDir()
			expectedImage := strings.Repeat("x", tc.imageSize)
			archive := createTestCheckpointArchive(
				t, tmpDir, "test.tar",
				"container-id-123456789", "custom-container-name", "crun",
				expectedImage, testCreated,
			)

			tasks, err := internal.CreateTasks([]string{archive}, []string{metadata.SpecDumpFile, metadata.ConfigDumpFile})
			if err != nil {
				t.Fatalf("CreateTasks failed: %v", err)
			}
			defer internal.CleanupTasks(tasks)

			meta, err := getTaskJSON(tasks)
			if err != nil {
				t.Fatalf("getTaskJSON returned error: %v", err)
			}
			if len(meta) != 1 {
				t.Fatalf("expected 1 metadata entry, got %d", len(meta))
			}
			m := meta[0]
			if m.ID != "container-id-123456789" {
				t.Errorf("ID mismatch: got %q, want %q", m.ID, "container-id-123456789")
			}
			if m.ContainerName != "custom-container-name" {
				t.Errorf("ContainerName mismatch: got %q, want %q", m.ContainerName, "custom-container-name")
			}
			if m.Runtime != "crun" {
				t.Errorf("Runtime mismatch: got %q, want %q", m.Runtime, "crun")
			}
			if m.Engine != "Podman" {
				t.Errorf("Engine mismatch: got %q, want %q", m.Engine, "Podman")
			}
			if m.Created != testCreated.Format(time.RFC3339) {
				t.Errorf("Created mismatch: got %q, want %q", m.Created, testCreated.Format(time.RFC3339))
			}
			if m.Image != expectedImage {
				t.Fatalf("image name truncated or corrupted: got %d bytes, want %d bytes",
					len(m.Image), len(expectedImage))
			}
		})
	}
}

func TestDiff_LargeOutputNoDeadlock(t *testing.T) {
	if runDiffTestSubprocess(t) {
		return
	}
	tmpDir := t.TempDir()
	largeImage := strings.Repeat("y", 80*1024)
	testCreated := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	archiveA := createTestCheckpointArchive(t, tmpDir, "cpA.tar", "c123456789012", "c-test", "crun", largeImage, testCreated)
	archiveB := createTestCheckpointArchive(t, tmpDir, "cpB.tar", "c123456789012", "c-test", "crun", largeImage, testCreated)

	cmd := Diff()
	cmd.SetArgs([]string{archiveA, archiveB})

	output := captureDiffOutput(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("checkpointctl diff failed: %v", err)
		}
	})

	if !strings.Contains(output, "Container: c-test") {
		t.Errorf("expected rendered output to contain container name, got:\n%s", output)
	}
	if !strings.Contains(output, "ID:        c123456789012") {
		t.Errorf("expected rendered output to contain container ID, got:\n%s", output)
	}
	if !strings.Contains(output, "Checkpoint comparison for container c-test") {
		t.Errorf("expected rendered output to contain summary, got:\n%s", output)
	}
}

// added and removed processes keep a stable PID order
func TestCompareProcessTreesDeterministicOrder(t *testing.T) {
	treeA := &internal.PsNode{PID: 1, Comm: "init"}
	treeB := &internal.PsNode{PID: 1, Comm: "init"}
	const n = 50
	for i := 0; i < n; i++ {
		treeB.Children = append(treeB.Children, internal.PsNode{PID: uint32(100 + i), Comm: "added"})
		treeA.Children = append(treeA.Children, internal.PsNode{PID: uint32(200 + i), Comm: "removed"})
	}

	first := compareProcessTrees(treeA, treeB)
	if len(first.Added) != n || len(first.Removed) != n {
		t.Fatalf("Expected %d added and %d removed processes, got %d and %d", n, n, len(first.Added), len(first.Removed))
	}
	for run := 0; run < 10; run++ {
		result := compareProcessTrees(treeA, treeB)
		for i := 0; i < n; i++ {
			if result.Added[i].PID != first.Added[i].PID {
				t.Fatalf("Added order changed between runs at index %d: %d != %d", i, result.Added[i].PID, first.Added[i].PID)
			}
			if result.Removed[i].PID != first.Removed[i].PID {
				t.Fatalf("Removed order changed between runs at index %d: %d != %d", i, result.Removed[i].PID, first.Removed[i].PID)
			}
		}
	}
	for i := 1; i < n; i++ {
		if first.Added[i-1].PID >= first.Added[i].PID {
			t.Errorf("Added processes not sorted by PID: %d >= %d", first.Added[i-1].PID, first.Added[i].PID)
		}
		if first.Removed[i-1].PID >= first.Removed[i].PID {
			t.Errorf("Removed processes not sorted by PID: %d >= %d", first.Removed[i-1].PID, first.Removed[i].PID)
		}
	}
}

// added and removed files keep a stable PID order
func TestCompareFileDescriptorsDeterministicOrder(t *testing.T) {
	const n = 50
	var fdsA, fdsB []FileDescriptorEntry
	for i := 0; i < n; i++ {
		fdsB = append(fdsB, FileDescriptorEntry{PID: 100 + i, OpenFiles: []OpenFile{{FD: "3", Path: "/added"}}})
		fdsA = append(fdsA, FileDescriptorEntry{PID: 200 + i, OpenFiles: []OpenFile{{FD: "3", Path: "/removed"}}})
	}

	first := compareFileDescriptors(fdsA, fdsB)
	if len(first.Added) != n || len(first.Removed) != n {
		t.Fatalf("Expected %d added and %d removed files, got %d and %d", n, n, len(first.Added), len(first.Removed))
	}
	for run := 0; run < 10; run++ {
		result := compareFileDescriptors(fdsA, fdsB)
		for i := 0; i < n; i++ {
			if result.Added[i] != first.Added[i] {
				t.Fatalf("Added order changed between runs at index %d: %+v != %+v", i, result.Added[i], first.Added[i])
			}
			if result.Removed[i] != first.Removed[i] {
				t.Fatalf("Removed order changed between runs at index %d: %+v != %+v", i, result.Removed[i], first.Removed[i])
			}
		}
	}
	for i := 1; i < n; i++ {
		if first.Added[i-1].PID >= first.Added[i].PID {
			t.Errorf("Added files not sorted by PID: %d >= %d", first.Added[i-1].PID, first.Added[i].PID)
		}
		if first.Removed[i-1].PID >= first.Removed[i].PID {
			t.Errorf("Removed files not sorted by PID: %d >= %d", first.Removed[i-1].PID, first.Removed[i].PID)
		}
	}
}

// added and removed sockets keep a stable PID order
func TestCompareSocketsDeterministicOrder(t *testing.T) {
	const n = 50
	var sksA, sksB []internal.SkNode
	for i := 0; i < n; i++ {
		sksB = append(sksB, internal.SkNode{PID: uint32(100 + i), OpenSockets: []internal.SocketNode{{
			Protocol: "TCP",
			Data:     internal.SkData{Type: "TCP", Source: "0.0.0.0", SourcePort: uint32(1000 + i)},
		}}})
		sksA = append(sksA, internal.SkNode{PID: uint32(200 + i), OpenSockets: []internal.SocketNode{{
			Protocol: "TCP",
			Data:     internal.SkData{Type: "TCP", Source: "0.0.0.0", SourcePort: uint32(2000 + i)},
		}}})
	}

	first := compareSockets(sksA, sksB)
	if len(first.Added) != n || len(first.Removed) != n {
		t.Fatalf("Expected %d added and %d removed sockets, got %d and %d", n, n, len(first.Added), len(first.Removed))
	}
	for run := 0; run < 10; run++ {
		result := compareSockets(sksA, sksB)
		for i := 0; i < n; i++ {
			if result.Added[i].PID != first.Added[i].PID || result.Added[i].SrcPort != first.Added[i].SrcPort {
				t.Fatalf("Added order changed between runs at index %d", i)
			}
			if result.Removed[i].PID != first.Removed[i].PID || result.Removed[i].SrcPort != first.Removed[i].SrcPort {
				t.Fatalf("Removed order changed between runs at index %d", i)
			}
		}
	}
	for i := 1; i < n; i++ {
		if first.Added[i-1].PID >= first.Added[i].PID {
			t.Errorf("Added sockets not sorted by PID: %d >= %d", first.Added[i-1].PID, first.Added[i].PID)
		}
		if first.Removed[i-1].PID >= first.Removed[i].PID {
			t.Errorf("Removed sockets not sorted by PID: %d >= %d", first.Removed[i-1].PID, first.Removed[i].PID)
		}
	}
}

func socketOrderSignature(sockets []SocketInfo) string {
	parts := make([]string, len(sockets))
	for i, s := range sockets {
		parts[i] = fmt.Sprintf("%d|%s|%s|%s|%d|%s|%s|%d", s.PID, s.Protocol, s.Type, s.Address, s.SrcPort, s.Source, s.Dest, s.DstPort)
	}
	return strings.Join(parts, ";")
}

// same PID, UNIX sockets differing only in address
func TestCompareSocketsDeterministicOrderSamePIDUnix(t *testing.T) {
	addresses := []string{"/run/f.sock", "/run/e.sock", "/run/d.sock", "/run/c.sock", "/run/b.sock", "/run/a.sock"}

	unixSocket := func(addr string) internal.SocketNode {
		return internal.SocketNode{Protocol: "UNIXSK", Data: internal.SkData{Type: "STREAM", Address: addr}}
	}
	sksA := []internal.SkNode{{PID: 100, OpenSockets: []internal.SocketNode{
		unixSocket("/run/a.sock"), unixSocket("/run/b.sock"), unixSocket("/run/c.sock"), unixSocket("/run/removed.sock"),
	}}}
	sksB := []internal.SkNode{{PID: 100, OpenSockets: []internal.SocketNode{
		unixSocket(addresses[0]), unixSocket(addresses[1]), unixSocket(addresses[2]),
		unixSocket(addresses[3]), unixSocket(addresses[4]), unixSocket(addresses[5]),
	}}}

	first := compareSockets(sksA, sksB)
	if len(first.Added) != 3 || len(first.Removed) != 1 || len(first.Unchanged) != 3 {
		t.Fatalf("Expected 3 added, 1 removed, 3 unchanged sockets, got %d, %d, %d",
			len(first.Added), len(first.Removed), len(first.Unchanged))
	}
	for run := 0; run < 20; run++ {
		result := compareSockets(sksA, sksB)
		if socketOrderSignature(result.Added) != socketOrderSignature(first.Added) {
			t.Fatalf("Added order changed between runs: %q vs %q", socketOrderSignature(result.Added), socketOrderSignature(first.Added))
		}
		if socketOrderSignature(result.Unchanged) != socketOrderSignature(first.Unchanged) {
			t.Fatalf("Unchanged order changed between runs: %q vs %q", socketOrderSignature(result.Unchanged), socketOrderSignature(first.Unchanged))
		}
	}
	wantAdded := []string{"/run/d.sock", "/run/e.sock", "/run/f.sock"}
	for i, want := range wantAdded {
		if first.Added[i].Address != want {
			t.Errorf("Added[%d].Address = %q, want %q", i, first.Added[i].Address, want)
		}
	}
	for i := 1; i < len(first.Unchanged); i++ {
		if first.Unchanged[i-1].Address >= first.Unchanged[i].Address {
			t.Errorf("Unchanged sockets not sorted by address: %q >= %q", first.Unchanged[i-1].Address, first.Unchanged[i].Address)
		}
	}
}

// same local endpoint, different TCP peers
func TestCompareSocketsDeterministicOrderSameLocalEndpoint(t *testing.T) {
	tcpSocket := func(dest string, dstPort uint32) internal.SocketNode {
		return internal.SocketNode{Protocol: "TCP", Data: internal.SkData{
			Type: "TCP", Source: "127.0.0.1", SourcePort: 8080, Dest: dest, DestPort: dstPort,
		}}
	}
	sksA := []internal.SkNode{{PID: 100, OpenSockets: []internal.SocketNode{
		tcpSocket("10.0.0.1", 9000), tcpSocket("10.0.0.2", 9000), tcpSocket("10.0.0.2", 9001), tcpSocket("10.0.0.3", 9000),
	}}}
	sksB := []internal.SkNode{{PID: 100, OpenSockets: []internal.SocketNode{
		tcpSocket("10.0.0.2", 9000), tcpSocket("10.0.0.2", 9001), tcpSocket("10.0.0.4", 9000),
	}}}

	first := compareSockets(sksA, sksB)
	if len(first.Added) != 1 || len(first.Removed) != 2 || len(first.Unchanged) != 2 {
		t.Fatalf("Expected 1 added, 2 removed, 2 unchanged sockets, got %d, %d, %d",
			len(first.Added), len(first.Removed), len(first.Unchanged))
	}
	for run := 0; run < 20; run++ {
		result := compareSockets(sksA, sksB)
		if socketOrderSignature(result.Removed) != socketOrderSignature(first.Removed) {
			t.Fatalf("Removed order changed between runs: %q vs %q", socketOrderSignature(result.Removed), socketOrderSignature(first.Removed))
		}
		if socketOrderSignature(result.Unchanged) != socketOrderSignature(first.Unchanged) {
			t.Fatalf("Unchanged order changed between runs: %q vs %q", socketOrderSignature(result.Unchanged), socketOrderSignature(first.Unchanged))
		}
	}
	wantRemoved := []struct {
		dest string
		port uint32
	}{{"10.0.0.1", 9000}, {"10.0.0.3", 9000}}
	for i, want := range wantRemoved {
		if first.Removed[i].Dest != want.dest || first.Removed[i].DstPort != want.port {
			t.Errorf("Removed[%d] = %s:%d, want %s:%d", i, first.Removed[i].Dest, first.Removed[i].DstPort, want.dest, want.port)
		}
	}
	if first.Unchanged[0].DstPort != 9000 || first.Unchanged[1].DstPort != 9001 {
		t.Errorf("Unchanged sockets not sorted by destination port: %d, %d", first.Unchanged[0].DstPort, first.Unchanged[1].DstPort)
	}
}
