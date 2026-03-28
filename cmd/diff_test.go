// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"
)

// compareSockets [empty inputs]
func TestCompareSocketsEmptyInputs(t *testing.T) {
	result := compareSockets([]SkNode{}, []SkNode{})

	if result == nil {
		t.Fatal("Expected non-nil result for empty inputs")
	}

	if len(result.Added) != 0 {
		t.Errorf("Expected 0 added sockets, got %d", len(result.Added))
	}

	if len(result.Removed) != 0 {
		t.Errorf("Expected 0 removed sockets, got %d", len(result.Removed))
	}

	if result.Unchanged != 0 {
		t.Errorf("Expected 0 unchanged sockets, got %d", result.Unchanged)
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

	if result.Unchanged != 0 {
		t.Errorf("Expected 0 unchanged sockets for nil inputs, got %d", result.Unchanged)
	}
}

// added sockets
func TestCompareSocketsAddedSockets(t *testing.T) {
	socketA := SkNode{
		PID: 1234,
		OpenSockets: []SocketNode{
			{
				Protocol: "TCP",
				Data: SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8080,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	socketB := SkNode{
		PID: 1234,
		OpenSockets: []SocketNode{
			{
				Protocol: "TCP",
				Data: SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8080,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
			{
				Protocol: "TCP",
				Data: SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8081,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	result := compareSockets([]SkNode{socketA}, []SkNode{socketB})

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if len(result.Added) != 1 {
		t.Errorf("Expected 1 added socket, got %d", len(result.Added))
	}

	if len(result.Removed) != 0 {
		t.Errorf("Expected 0 removed sockets, got %d", len(result.Removed))
	}

	if result.Unchanged != 1 {
		t.Errorf("Expected 1 unchanged socket, got %d", result.Unchanged)
	}

	// verify added socket is the correct one (port 8081)
	if result.Added[0].SrcPort != 8081 {
		t.Errorf("Expected added socket to have port 8081, got %d", result.Added[0].SrcPort)
	}
}

// removed sockets
func TestCompareSocketsRemovedSockets(t *testing.T) {
	socketA := SkNode{
		PID: 1234,
		OpenSockets: []SocketNode{
			{
				Protocol: "TCP",
				Data: SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8080,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
			{
				Protocol: "TCP",
				Data: SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8081,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	socketB := SkNode{
		PID: 1234,
		OpenSockets: []SocketNode{
			{
				Protocol: "TCP",
				Data: SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 8081,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	result := compareSockets([]SkNode{socketA}, []SkNode{socketB})

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if len(result.Added) != 0 {
		t.Errorf("Expected 0 added sockets, got %d", len(result.Added))
	}

	if len(result.Removed) != 1 {
		t.Errorf("Expected 1 removed socket, got %d", len(result.Removed))
	}

	if result.Unchanged != 1 {
		t.Errorf("Expected 1 unchanged socket, got %d", result.Unchanged)
	}

	// verify removed socket is the correct one (port 8080)
	if result.Removed[0].SrcPort != 8080 {
		t.Errorf("Expected removed socket to have port 8080, got %d", result.Removed[0].SrcPort)
	}
}

// comparing sockets from different processes
func TestCompareSocketsMultiplePIDs(t *testing.T) {
	socketPID1 := SkNode{
		PID: 1,
		OpenSockets: []SocketNode{
			{
				Protocol: "TCP",
				Data: SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 80,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	socketPID2 := SkNode{
		PID: 2,
		OpenSockets: []SocketNode{
			{
				Protocol: "TCP",
				Data: SkData{
					Type:       "TCP",
					Source:     "0.0.0.0",
					SourcePort: 443,
					Dest:       "0.0.0.0",
					DestPort:   0,
				},
			},
		},
	}

	result := compareSockets([]SkNode{socketPID1, socketPID2}, []SkNode{socketPID1})

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
			Unchanged: 0,
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
	socketA := SkNode{
		PID: 1234,
		OpenSockets: []SocketNode{
			{
				Protocol: "UNIX",
				Data: SkData{
					Type:    "UNIX",
					Address: "", // Empty address
				},
			},
		},
	}

	socketB := SkNode{
		PID: 1234,
		OpenSockets: []SocketNode{
			{
				Protocol: "UNIX",
				Data: SkData{
					Type:    "UNIX",
					Address: "", //
				},
			},
		},
	}

	result := compareSockets([]SkNode{socketA}, []SkNode{socketB})

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

	if result.Unchanged != 1 {
		t.Errorf("Expected 1 unchanged socket, got %d", result.Unchanged)
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
