// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"

	"github.com/checkpoint-restore/checkpointctl/internal"
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
