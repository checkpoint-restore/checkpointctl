package internal

import (
	"encoding/json"
	"fmt"
	"sort"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/xlab/treeprint"
)

func RenderTreeView(tasks []Task) error {
	data, err := CollectCheckpointData(tasks)
	if err != nil {
		return err
	}

	for _, node := range data {
		tree := buildTreeFromDisplayNode(node)
		fmt.Printf("\nDisplaying container checkpoint tree view from %s\n\n", node.checkpointFilePath)
		fmt.Println(tree.String())
	}

	return nil
}

func buildTreeFromDisplayNode(node DisplayNode) treeprint.Tree {
	name := node.ContainerName
	if name == "" {
		name = "Container"
	}
	tree := treeprint.NewWithRoot(name)

	tree.AddBranch(fmt.Sprintf("Image: %s", node.Image))
	tree.AddBranch(fmt.Sprintf("ID: %s", node.ID))
	tree.AddBranch(fmt.Sprintf("Runtime: %s", node.Runtime))
	tree.AddBranch(fmt.Sprintf("Created: %s", node.Created))
	if node.Checkpointed != "" {
		tree.AddBranch(fmt.Sprintf("Checkpointed: %s", node.Checkpointed))
	}
	tree.AddBranch(fmt.Sprintf("Engine: %s", node.Engine))

	if node.IP != "" {
		tree.AddBranch(fmt.Sprintf("IP: %s", node.IP))
	}
	if node.MAC != "" {
		tree.AddBranch(fmt.Sprintf("MAC: %s", node.MAC))
	}

	checkpointSize := tree.AddBranch(fmt.Sprintf("Checkpoint size: %s", metadata.ByteToString(node.CheckpointSize.TotalSize)))
	if node.CheckpointSize.MemoryPagesSize != 0 {
		checkpointSize.AddNode(fmt.Sprintf("Memory pages size: %s", metadata.ByteToString(node.CheckpointSize.MemoryPagesSize)))
	}
	if node.CheckpointSize.AmdGpuMemoryPagesSize != 0 {
		checkpointSize.AddNode(fmt.Sprintf("AMD GPU memory pages size: %s", metadata.ByteToString(node.CheckpointSize.AmdGpuMemoryPagesSize)))
	}

	if node.CheckpointSize.RootFsDiffSize != 0 {
		tree.AddBranch(fmt.Sprintf("Root FS diff size: %s", metadata.ByteToString(node.CheckpointSize.RootFsDiffSize)))
	}

	if node.CriuDumpStatistics != nil {
		addStatsNodeToTree(tree, node.CriuDumpStatistics)
	}

	if node.Metadata != nil {
		addMetadataNodeToTree(tree, node.Metadata)
	}

	if node.ProcessTree != nil {
		addPsNodeToTree(tree, node.ProcessTree, node.FileDescriptors, node.Sockets)
	}

	if len(node.Mounts) > 0 {
		addMountNodesToTree(tree, node.Mounts)
	}

	return tree
}

func addStatsNodeToTree(tree treeprint.Tree, stats *StatsNode) {
	statsTree := tree.AddBranch("CRIU dump statistics")
	statsTree.AddBranch(fmt.Sprintf("Freezing time: %s", FormatTime(stats.FreezingTime)))
	statsTree.AddBranch(fmt.Sprintf("Frozen time: %s", FormatTime(stats.FrozenTime)))
	statsTree.AddBranch(fmt.Sprintf("Memdump time: %s", FormatTime(stats.MemdumpTime)))
	statsTree.AddBranch(fmt.Sprintf("Memwrite time: %s", FormatTime(stats.MemwriteTime)))
	statsTree.AddBranch(fmt.Sprintf("Pages scanned: %d", stats.PagesScanned))
	statsTree.AddBranch(fmt.Sprintf("Pages written: %d", stats.PagesWritten))
}

func addMetadataNodeToTree(tree treeprint.Tree, meta *MetadataNode) {
	podTree := tree.AddBranch("Metadata")
	if meta.PodName != "" {
		podTree.AddBranch(fmt.Sprintf("Pod name: %s", meta.PodName))
	}
	if meta.KubernetesNamespace != "" {
		podTree.AddBranch(fmt.Sprintf("Kubernetes namespace: %s", meta.KubernetesNamespace))
	}
	if len(meta.Annotations) > 0 {
		annotationTree := podTree.AddBranch("Annotations")
		for key, value := range meta.Annotations {
			switch key {
			case "io.kubernetes.cri-o.Labels",
				"io.kubernetes.cri-o.Annotations",
				"io.kubernetes.cri-o.Metadata",
				"kubectl.kubernetes.io/last-applied-configuration":
				// We know that some annotations contain a JSON string we can pretty print
				local := make(map[string]interface{})
				if err := json.Unmarshal([]byte(value), &local); err != nil {
					continue
				}
				localTree := annotationTree.AddBranch(key)
				addMapToTree(localTree, local)
			case "io.kubernetes.cri-o.Volumes":
				var local []mountAnnotations
				if err := json.Unmarshal([]byte(value), &local); err != nil {
					continue
				}
				localTree := annotationTree.AddBranch(key)
				for _, mount := range local {
					containerPath := localTree.AddBranch(mount.ContainerPath)
					containerPath.AddBranch(fmt.Sprintf("host path: %s", mount.HostPath))
					containerPath.AddBranch(fmt.Sprintf("read-only: %t", mount.Readonly))
					containerPath.AddBranch(fmt.Sprintf("selinux relabel: %t", mount.SelinuxRelabel))
					containerPath.AddBranch(fmt.Sprintf("recursive read-only: %t", mount.RecursiveReadOnly))
					containerPath.AddBranch(fmt.Sprintf("propagation: %d", mount.Propagation))
				}
			default:
				annotationTree.AddBranch(fmt.Sprintf("%s: %s", key, value))
			}
		}
	}
}

func addPsNodeToTree(tree treeprint.Tree, ps *PsNode, fds []FdNode, sks []SkNode) {
	psTreeNode := tree.AddBranch("Process tree")
	addPsNodeBranch(psTreeNode, ps, fds, sks)
}

func addPsNodeBranch(tree treeprint.Tree, ps *PsNode, fds []FdNode, sks []SkNode) {
	var metaBranchTag string
	// "Alive" is what CRIU returns for running processes
	if ps.TaskState == "" || ps.TaskState == "Alive" || ps.TaskState == "Running" {
		metaBranchTag = fmt.Sprintf("%d", ps.PID)
	} else {
		metaBranchTag = fmt.Sprintf("%d (%s)", ps.PID, ps.TaskState)
	}

	// Use cmdline if available (when --ps-tree-cmd is used), otherwise use command
	displayName := ps.Comm
	if ps.Cmdline != "" {
		displayName = ps.Cmdline
	}
	node := tree.AddMetaBranch(metaBranchTag, displayName)

	// Skip adding children/files/sockets for dead/zombie processes
	// "Alive" and "Stopped" are valid states from CRIU that should show file descriptors
	if ps.TaskState != "" && ps.TaskState != "Alive" && ps.TaskState != "Running" && ps.TaskState != "Stopped" {
		return
	}

	// Add environment variables if present
	if len(ps.EnvVars) > 0 {
		envTree := node.AddBranch("Environment variables")
		for key, value := range ps.EnvVars {
			envTree.AddBranch(fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Add file descriptors for this process
	for _, fd := range fds {
		if fd.PID != ps.PID {
			continue
		}
		if len(fd.OpenFiles) > 0 {
			filesTree := node.AddBranch("Open files")
			for _, file := range fd.OpenFiles {
				filesTree.AddMetaBranch(file.FD, file.Path)
			}
		}
	}

	// Add sockets for this process
	for _, sk := range sks {
		if sk.PID != ps.PID {
			continue
		}
		if len(sk.OpenSockets) > 0 {
			socketsTree := node.AddBranch("Open sockets")
			for _, socket := range sk.OpenSockets {
				protocol, data := formatSocketForTree(socket)
				socketsTree.AddMetaBranch(protocol, data)
			}
		}
	}

	// Sort children by PID for consistent output
	children := make([]PsNode, len(ps.Children))
	copy(children, ps.Children)
	sort.Slice(children, func(i, j int) bool {
		return children[i].PID < children[j].PID
	})

	// Recursively add children
	for i := range children {
		addPsNodeBranch(node, &children[i], fds, sks)
	}
}

func formatSocketForTree(socket SocketNode) (protocol, data string) {
	protocol = socket.Protocol
	skData := socket.Data

	switch skData.Type {
	case "UNIX":
		protocol = fmt.Sprintf("UNIX (%s)", socket.Protocol)
		data = skData.Address
		if len(data) == 0 {
			data = "@"
		}
	case "TCP", "UDP":
		if skData.Type == "TCP" && skData.State != "" {
			protocol = fmt.Sprintf("%s (%s)", skData.Type, skData.State)
		} else {
			protocol = skData.Type
		}
		data = fmt.Sprintf(
			"%s:%d -> %s:%d (↑ %s ↓ %s)",
			skData.Source, skData.SourcePort,
			skData.Dest, skData.DestPort,
			skData.SendBuf, skData.RecvBuf,
		)
	default:
		// PACKETSK, NETLINKSK
		data = fmt.Sprintf("↑ %s ↓ %s", skData.SendBuf, skData.RecvBuf)
	}

	return protocol, data
}

func addMountNodesToTree(tree treeprint.Tree, mounts []MountNode) {
	mountsTree := tree.AddBranch("Overview of mounts")
	for _, mount := range mounts {
		mountTree := mountsTree.AddBranch(fmt.Sprintf("Destination: %s", mount.Destination))
		mountTree.AddBranch(fmt.Sprintf("Type: %s", mount.Type))
		mountTree.AddBranch(fmt.Sprintf("Source: %s", mount.Source))
	}
}

// Taken from the CRI API
type mountAnnotations struct {
	ContainerPath     string `json:"container_path,omitempty"`
	HostPath          string `json:"host_path,omitempty"`
	Readonly          bool   `json:"readonly,omitempty"`
	SelinuxRelabel    bool   `json:"selinux_relabel,omitempty"`
	Propagation       int    `json:"propagation,omitempty"`
	UidMappings       []*int `json:"uidMappings,omitempty"`
	GidMappings       []*int `json:"gidMappings,omitempty"`
	RecursiveReadOnly bool   `json:"recursive_read_only,omitempty"`
}

func addMapToTree(tree treeprint.Tree, data map[string]interface{}) {
	for key, value := range data {
		switch v := value.(type) {
		case map[string]interface{}:
			// Recursively add nested maps
			subTree := tree.AddBranch(fmt.Sprintf("%s:", key))
			addMapToTree(subTree, v)
		case []interface{}:
			// Handle arrays recursively
			arrayTree := tree.AddBranch(fmt.Sprintf("%s: ", key))
			addArrayToTree(arrayTree, v)
		default:
			tree.AddBranch(fmt.Sprintf("%s: %v", key, v))
		}
	}
}

func addArrayToTree(tree treeprint.Tree, data []interface{}) {
	for i, value := range data {
		switch v := value.(type) {
		case map[string]interface{}:
			// Recursively add maps inside arrays
			subTree := tree.AddBranch(fmt.Sprintf("[%d]:", i))
			addMapToTree(subTree, v)
		case []interface{}:
			// Recursively add arrays inside arrays
			subArrayTree := tree.AddBranch(fmt.Sprintf("[%d]: ", i))
			addArrayToTree(subArrayTree, v)
		default:
			tree.AddBranch(fmt.Sprintf("[%d]: %v", i, v))
		}
	}
}
