package internal

import (
	"reflect"
	"testing"

	"github.com/checkpoint-restore/go-criu/v7/crit"
	spec "github.com/opencontainers/runtime-spec/specs-go"
)

func TestGetUnixSkData(t *testing.T) {
	socket := &crit.Socket{
		SrcAddr: "test_addr",
	}

	expectedData := SkData{
		Type:    "UNIX",
		Address: "test_addr",
	}

	result := getUnixSkData(socket)

	if !reflect.DeepEqual(result, expectedData) {
		t.Errorf("Expected %v, but got %v", expectedData, result)
	}
}

func TestGetInetSkData(t *testing.T) {
	socket := &crit.Socket{
		Protocol: "tcp",
		State:    "ESTABLISHED",
		SrcAddr:  "src_addr",
		SrcPort:  1234,
		DestAddr: "dest_addr",
		DestPort: 5678,
		SendBuf:  "send_buffer",
		RecvBuf:  "recv_buffer",
	}

	expectedData := SkData{
		Type:       "tcp",
		State:      "ESTABLISHED",
		Source:     "src_addr",
		SourcePort: 1234,
		Dest:       "dest_addr",
		DestPort:   5678,
		SendBuf:    "send_buffer",
		RecvBuf:    "recv_buffer",
	}

	result := getInetSkData(socket)

	if !reflect.DeepEqual(result, expectedData) {
		t.Errorf("Expected %v, but got %v", expectedData, result)
	}
}

func TestGetPacketSkData(t *testing.T) {
	socket := &crit.Socket{
		SendBuf: "send_buffer",
		RecvBuf: "recv_buffer",
	}

	expectedData := SkData{
		SendBuf: "send_buffer",
		RecvBuf: "recv_buffer",
	}

	result := getPacketSkData(socket)

	if !reflect.DeepEqual(result, expectedData) {
		t.Errorf("Expected %v, but got %v", expectedData, result)
	}
}

func TestGetNetlinkSkData(t *testing.T) {
	socket := &crit.Socket{
		SendBuf: "send_buffer",
		RecvBuf: "recv_buffer",
	}

	expectedData := SkData{
		SendBuf: "send_buffer",
		RecvBuf: "recv_buffer",
	}

	result := getNetlinkSkData(socket)

	if !reflect.DeepEqual(result, expectedData) {
		t.Errorf("Expected %v, but got %v", expectedData, result)
	}
}

func TestBuildJSONMounts(t *testing.T) {
	specDump := &spec.Spec{
		Mounts: []spec.Mount{
			{Destination: "/mnt1", Type: "bind", Source: "/src1"},
			{Destination: "/mnt2", Type: "bind", Source: "/src2"},
		},
	}

	expectedMounts := []MountNode{
		{Destination: "/mnt1", Type: "bind", Source: "/src1"},
		{Destination: "/mnt2", Type: "bind", Source: "/src2"},
	}

	result := buildJSONMounts(specDump)

	if !reflect.DeepEqual(result, expectedMounts) {
		t.Errorf("Expected %v, but got %v", expectedMounts, result)
	}
}

func TestBuildJSONPsTree(t *testing.T) {
	psTree := &crit.PsTree{
		PID:  1,
		Comm: "root",
		Children: []*crit.PsTree{
			{PID: 2, Comm: "child1"},
			{PID: 3, Comm: "child2"},
		},
	}

	expectedPsTree := PsNode{
		PID:  1,
		Comm: "root",
		Children: []PsNode{
			{PID: 2, Comm: "child1"},
			{PID: 3, Comm: "child2"},
		},
	}

	result, err := buildJSONPsTree(psTree, "checkpointOutputDir")
	if err != nil {
		t.Errorf("Error building JSON process tree: %v", err)
	}

	if !reflect.DeepEqual(result, expectedPsTree) {
		t.Errorf("Expected %v, but got %v", expectedPsTree, result)
	}
}

func TestBuildJSONPsNode(t *testing.T) {
	psTree := &crit.PsTree{
		PID:  1,
		Comm: "root",
		Children: []*crit.PsTree{
			{PID: 2, Comm: "child1"},
			{PID: 3, Comm: "child2"},
		},
	}

	expectedPsNode := PsNode{
		PID:  1,
		Comm: "root",
		Children: []PsNode{
			{PID: 2, Comm: "child1"},
			{PID: 3, Comm: "child2"},
		},
	}

	result, err := buildJSONPsNode(psTree, "checkpointOutputDir")
	if err != nil {
		t.Errorf("Error building JSON process node: %v", err)
	}

	if !reflect.DeepEqual(result, expectedPsNode) {
		t.Errorf("Expected %v, but got %v", expectedPsNode, result)
	}
}

func TestBuildJSONFds(t *testing.T) {
	fds := []*crit.Fd{
		{
			PId: 1,
			Files: []*crit.File{
				{Type: "file", Fd: "1", Path: "/path1"},
				{Type: "socket", Fd: "2", Path: "/path2"},
			},
		},
		{
			PId: 2,
			Files: []*crit.File{
				{Type: "file", Fd: "3", Path: "/path3"},
				{Type: "socket", Fd: "4", Path: "/path4"},
			},
		},
	}

	expectedFds := []FdNode{
		{
			PID: 1,
			OpenFiles: []OpenFileNode{
				{Type: "file", FD: "file 1", Path: "/path1"},
				{Type: "socket", FD: "socket 2", Path: "/path2"},
			},
		},
		{
			PID: 2,
			OpenFiles: []OpenFileNode{
				{Type: "file", FD: "file 3", Path: "/path3"},
				{Type: "socket", FD: "socket 4", Path: "/path4"},
			},
		},
	}

	result := buildJSONFds(fds)

	if !reflect.DeepEqual(result, expectedFds) {
		t.Errorf("Expected %v, but got %v", expectedFds, result)
	}
}

func TestBuildJSONSks(t *testing.T) {
	mockSks := []*crit.Sk{
		{
			PId: 123,
			Sockets: []*crit.Socket{
				{
					FdType:   "INETSK",
					Protocol: "tcp",
					State:    "ESTABLISHED",
					SrcAddr:  "192.168.1.1",
					SrcPort:  8080,
					DestAddr: "192.168.1.2",
					DestPort: 80,
					SendBuf:  "100KB",
					RecvBuf:  "50KB",
				},
			},
		},
	}

	expectedResult := []SkNode{
		{
			PID: 123,
			OpenSockets: []SocketNode{
				{
					Protocol: "tcp",
					Data: SkData{
						Type:       "tcp",
						State:      "ESTABLISHED",
						Source:     "192.168.1.1",
						SourcePort: 8080,
						Dest:       "192.168.1.2",
						DestPort:   80,
						SendBuf:    "100KB",
						RecvBuf:    "50KB",
					},
				},
			},
		},
	}

	result, err := buildJSONSks(mockSks)
	if err != nil {
		t.Errorf("Error building JSON Sks: %v", err)
		return
	}

	if !reflect.DeepEqual(result, expectedResult) {
		t.Errorf("BuildJSONSks did not produce the expected result.\nExpected:\n%v\nGot:\n%v", expectedResult, result)
	}
}
