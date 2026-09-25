// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	defer f.Close()

	tw := tar.NewWriter(f)
	defer tw.Close()

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

func TestGetTaskJSON_LargeOutput(t *testing.T) {
	testCreated := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	testCases := []struct {
		name      string
		imageSize int
	}{
		{
			name:      "serialized output exceeding effective host pipe capacity (~75 KiB)",
			imageSize: 75 * 1024,
		},
		{
			name:      "serialized output well above 1 MiB (~1.5 MiB)",
			imageSize: 1500 * 1024,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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

			type result struct {
				meta []CheckpointMetadata
				err  error
			}
			ch := make(chan result, 1)

			go func() {
				m, e := getTaskJSON(tasks)
				ch <- result{meta: m, err: e}
			}()

			select {
			case <-time.After(10 * time.Second):
				t.Fatal("getTaskJSON deadlocked/timed out while processing large output")
			case res := <-ch:
				if res.err != nil {
					t.Fatalf("getTaskJSON returned error: %v", res.err)
				}
				if len(res.meta) != 1 {
					t.Fatalf("expected 1 metadata entry, got %d", len(res.meta))
				}
				m := res.meta[0]
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
			}
		})
	}
}

func TestDiff_LargeOutputNoDeadlock(t *testing.T) {
	tmpDir := t.TempDir()
	largeImage := strings.Repeat("y", 80*1024) // Exceeds default host pipe capacity
	testCreated := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	archiveA := createTestCheckpointArchive(t, tmpDir, "cpA.tar", "c123456789012", "c-test", "crun", largeImage, testCreated)
	archiveB := createTestCheckpointArchive(t, tmpDir, "cpB.tar", "c123456789012", "c-test", "crun", largeImage, testCreated)

	cmd := Diff()
	cmd.SetArgs([]string{archiveA, archiveB})

	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	select {
	case <-time.After(10 * time.Second):
		t.Fatal("checkpointctl diff deadlocked on output exceeding pipe capacity")
	case err := <-done:
		if err != nil {
			t.Fatalf("checkpointctl diff failed: %v", err)
		}
	}
}
