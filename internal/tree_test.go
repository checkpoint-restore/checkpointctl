package internal

import (
	"strings"
	"testing"

	"github.com/xlab/treeprint"
)

func TestBuildTreeFromDisplayNode(t *testing.T) {
	node := DisplayNode{
		ContainerName: "test-container",
		Image:         "test-image:latest",
		ID:            "abc123",
		Runtime:       "runc",
		Created:       "2024-01-01T00:00:00Z",
		Engine:        "Podman",
		CheckpointSize: CheckpointSize{
			TotalSize: 1024,
		},
	}

	tree := buildTreeFromDisplayNode(node)
	result := tree.String()

	expectedStrings := []string{
		"test-container",
		"Image: test-image:latest",
		"ID: abc123",
		"Runtime: runc",
		"Created: 2024-01-01T00:00:00Z",
		"Engine: Podman",
		"Checkpoint size:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}

func TestBuildTreeFromDisplayNodeWithEmptyName(t *testing.T) {
	node := DisplayNode{
		ContainerName: "",
		Image:         "test-image",
		ID:            "123",
		Runtime:       "runc",
		Created:       "2024-01-01T00:00:00Z",
		Engine:        "Podman",
		CheckpointSize: CheckpointSize{
			TotalSize: 1024,
		},
	}

	tree := buildTreeFromDisplayNode(node)
	result := tree.String()

	if !strings.Contains(result, "Container") {
		t.Errorf("Expected tree root to be 'Container' when name is empty, but got:\n%s", result)
	}
}

func TestBuildTreeFromDisplayNodeWithOptionalFields(t *testing.T) {
	node := DisplayNode{
		ContainerName: "test-container",
		Image:         "test-image",
		ID:            "123",
		Runtime:       "runc",
		Created:       "2024-01-01T00:00:00Z",
		Checkpointed:  "2024-01-02T00:00:00Z",
		Engine:        "Podman",
		IP:            "192.168.1.1",
		MAC:           "00:11:22:33:44:55",
		CheckpointSize: CheckpointSize{
			TotalSize:             2048,
			MemoryPagesSize:       1024,
			AmdGpuMemoryPagesSize: 512,
			RootFsDiffSize:        256,
		},
	}

	tree := buildTreeFromDisplayNode(node)
	result := tree.String()

	expectedStrings := []string{
		"Checkpointed: 2024-01-02T00:00:00Z",
		"IP: 192.168.1.1",
		"MAC: 00:11:22:33:44:55",
		"Memory pages size:",
		"AMD GPU memory pages size:",
		"Root FS diff size:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}

func TestAddStatsNodeToTree(t *testing.T) {
	tree := treeprint.New()
	stats := &StatsNode{
		FreezingTime: 1000,
		FrozenTime:   2000,
		MemdumpTime:  3000,
		MemwriteTime: 4000,
		PagesScanned: 100,
		PagesWritten: 50,
	}

	addStatsNodeToTree(tree, stats)
	result := tree.String()

	expectedStrings := []string{
		"CRIU dump statistics",
		"Freezing time:",
		"Frozen time:",
		"Memdump time:",
		"Memwrite time:",
		"Pages scanned: 100",
		"Pages written: 50",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}

func TestAddMetadataNodeToTree(t *testing.T) {
	tree := treeprint.New()
	meta := &MetadataNode{
		PodName:             "test-pod",
		KubernetesNamespace: "test-namespace",
		Annotations: map[string]string{
			"custom-annotation": "custom-value",
		},
	}

	addMetadataNodeToTree(tree, meta)
	result := tree.String()

	expectedStrings := []string{
		"Metadata",
		"Pod name: test-pod",
		"Kubernetes namespace: test-namespace",
		"Annotations",
		"custom-annotation: custom-value",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}

func TestAddMetadataNodeToTreeWithJSONAnnotation(t *testing.T) {
	tree := treeprint.New()
	meta := &MetadataNode{
		Annotations: map[string]string{
			"io.kubernetes.cri-o.Labels": `{"app": "test", "version": "1.0"}`,
		},
	}

	addMetadataNodeToTree(tree, meta)
	result := tree.String()

	// Should contain parsed JSON fields
	if !strings.Contains(result, "io.kubernetes.cri-o.Labels") {
		t.Errorf("Expected tree to contain annotation key, but it didn't.\nTree:\n%s", result)
	}
	if !strings.Contains(result, "app:") {
		t.Errorf("Expected tree to contain parsed JSON field 'app', but it didn't.\nTree:\n%s", result)
	}
}

func TestAddPsNodeToTree(t *testing.T) {
	tree := treeprint.New()
	ps := &PsNode{
		PID:       1,
		Comm:      "init",
		TaskState: "Alive",
		Children: []PsNode{
			{PID: 2, Comm: "child1", TaskState: "Alive"},
			{PID: 3, Comm: "child2", TaskState: "Stopped"},
		},
	}

	addPsNodeToTree(tree, ps, nil, nil)
	result := tree.String()

	expectedStrings := []string{
		"Process tree",
		"[1]  init",
		"[2]  child1",
		"[3 (Stopped)]  child2",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}

func TestAddPsNodeToTreeWithDeadProcess(t *testing.T) {
	tree := treeprint.New()
	ps := &PsNode{
		PID:       1,
		Comm:      "zombie",
		TaskState: "Dead",
	}

	addPsNodeToTree(tree, ps, nil, nil)
	result := tree.String()

	if !strings.Contains(result, "[1 (Dead)]  zombie") {
		t.Errorf("Expected tree to show dead process with state, but got:\n%s", result)
	}
}

func TestAddPsNodeToTreeWithCmdline(t *testing.T) {
	tree := treeprint.New()
	ps := &PsNode{
		PID:       1,
		Comm:      "bash",
		Cmdline:   "/bin/bash --login",
		TaskState: "Alive",
	}

	addPsNodeToTree(tree, ps, nil, nil)
	result := tree.String()

	if !strings.Contains(result, "/bin/bash --login") {
		t.Errorf("Expected tree to show cmdline instead of comm, but got:\n%s", result)
	}
}

func TestAddPsNodeToTreeWithEnvVars(t *testing.T) {
	tree := treeprint.New()
	ps := &PsNode{
		PID:       1,
		Comm:      "test",
		TaskState: "Alive",
		EnvVars: map[string]string{
			"HOME": "/root",
			"PATH": "/usr/bin",
		},
	}

	addPsNodeToTree(tree, ps, nil, nil)
	result := tree.String()

	expectedStrings := []string{
		"Environment variables",
		"HOME=/root",
		"PATH=/usr/bin",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}

func TestAddPsNodeToTreeWithFiles(t *testing.T) {
	tree := treeprint.New()
	ps := &PsNode{
		PID:       1,
		Comm:      "test",
		TaskState: "Alive",
	}
	fds := []FdNode{
		{
			PID: 1,
			OpenFiles: []OpenFileNode{
				{Type: "REG", FD: "REG 0", Path: "/etc/passwd"},
				{Type: "REG", FD: "REG 1", Path: "/etc/hosts"},
			},
		},
	}

	addPsNodeToTree(tree, ps, fds, nil)
	result := tree.String()

	expectedStrings := []string{
		"Open files",
		"[REG 0]  /etc/passwd",
		"[REG 1]  /etc/hosts",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}

func TestAddPsNodeToTreeWithSockets(t *testing.T) {
	tree := treeprint.New()
	ps := &PsNode{
		PID:       1,
		Comm:      "test",
		TaskState: "Alive",
	}
	sks := []SkNode{
		{
			PID: 1,
			OpenSockets: []SocketNode{
				{
					Protocol: "TCP",
					Data: SkData{
						Type:       "TCP",
						State:      "ESTABLISHED",
						Source:     "127.0.0.1",
						SourcePort: 8080,
						Dest:       "127.0.0.1",
						DestPort:   80,
						SendBuf:    "1KB",
						RecvBuf:    "2KB",
					},
				},
			},
		},
	}

	addPsNodeToTree(tree, ps, nil, sks)
	result := tree.String()

	expectedStrings := []string{
		"Open sockets",
		"TCP (ESTABLISHED)",
		"127.0.0.1:8080",
		"127.0.0.1:80",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}

func TestFormatSocketForTreeUnix(t *testing.T) {
	socket := SocketNode{
		Protocol: "STREAM",
		Data: SkData{
			Type:    "UNIX",
			Address: "/var/run/test.sock",
		},
	}

	protocol, data := formatSocketForTree(socket)

	if protocol != "UNIX (STREAM)" {
		t.Errorf("Expected protocol 'UNIX (STREAM)', got %q", protocol)
	}
	if data != "/var/run/test.sock" {
		t.Errorf("Expected data '/var/run/test.sock', got %q", data)
	}
}

func TestFormatSocketForTreeUnixEmptyAddress(t *testing.T) {
	socket := SocketNode{
		Protocol: "STREAM",
		Data: SkData{
			Type:    "UNIX",
			Address: "",
		},
	}

	_, data := formatSocketForTree(socket)

	if data != "@" {
		t.Errorf("Expected data '@' for empty UNIX address, got %q", data)
	}
}

func TestFormatSocketForTreeTCP(t *testing.T) {
	socket := SocketNode{
		Protocol: "TCP",
		Data: SkData{
			Type:       "TCP",
			State:      "LISTEN",
			Source:     "0.0.0.0",
			SourcePort: 443,
			Dest:       "0.0.0.0",
			DestPort:   0,
			SendBuf:    "64KB",
			RecvBuf:    "32KB",
		},
	}

	protocol, data := formatSocketForTree(socket)

	if protocol != "TCP (LISTEN)" {
		t.Errorf("Expected protocol 'TCP (LISTEN)', got %q", protocol)
	}
	if !strings.Contains(data, "0.0.0.0:443") {
		t.Errorf("Expected data to contain source address, got %q", data)
	}
	if !strings.Contains(data, "64KB") {
		t.Errorf("Expected data to contain send buffer, got %q", data)
	}
}

func TestFormatSocketForTreeUDP(t *testing.T) {
	socket := SocketNode{
		Protocol: "UDP",
		Data: SkData{
			Type:       "UDP",
			Source:     "192.168.1.1",
			SourcePort: 53,
			Dest:       "8.8.8.8",
			DestPort:   53,
			SendBuf:    "8KB",
			RecvBuf:    "8KB",
		},
	}

	protocol, data := formatSocketForTree(socket)

	if protocol != "UDP" {
		t.Errorf("Expected protocol 'UDP', got %q", protocol)
	}
	if !strings.Contains(data, "192.168.1.1:53") {
		t.Errorf("Expected data to contain source, got %q", data)
	}
}

func TestFormatSocketForTreePacket(t *testing.T) {
	socket := SocketNode{
		Protocol: "PACKET",
		Data: SkData{
			SendBuf: "16KB",
			RecvBuf: "16KB",
		},
	}

	_, data := formatSocketForTree(socket)

	if !strings.Contains(data, "16KB") {
		t.Errorf("Expected data to contain buffer sizes, got %q", data)
	}
	if !strings.Contains(data, "↑") || !strings.Contains(data, "↓") {
		t.Errorf("Expected data to contain arrows, got %q", data)
	}
}

func TestAddMountNodesToTree(t *testing.T) {
	tree := treeprint.New()
	mounts := []MountNode{
		{Destination: "/mnt/data", Type: "bind", Source: "/host/data"},
		{Destination: "/tmp", Type: "tmpfs", Source: "tmpfs"},
	}

	addMountNodesToTree(tree, mounts)
	result := tree.String()

	expectedStrings := []string{
		"Overview of mounts",
		"Destination: /mnt/data",
		"Type: bind",
		"Source: /host/data",
		"Destination: /tmp",
		"Type: tmpfs",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}

func TestAddMapToTree(t *testing.T) {
	tree := treeprint.New()
	data := map[string]interface{}{
		"string_key": "string_value",
		"nested_map": map[string]interface{}{
			"inner_key": "inner_value",
		},
		"array_key": []interface{}{"item1", "item2"},
	}

	addMapToTree(tree, data)
	result := tree.String()

	expectedStrings := []string{
		"string_key: string_value",
		"nested_map:",
		"inner_key: inner_value",
		"array_key:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}

func TestAddArrayToTree(t *testing.T) {
	tree := treeprint.New()
	data := []interface{}{
		"simple_item",
		map[string]interface{}{"key": "value"},
		[]interface{}{"nested_item"},
	}

	addArrayToTree(tree, data)
	result := tree.String()

	expectedStrings := []string{
		"[0]: simple_item",
		"[1]:",
		"key: value",
		"[2]:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected tree to contain %q, but it didn't.\nTree:\n%s", expected, result)
		}
	}
}
