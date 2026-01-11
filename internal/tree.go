package internal

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v8/crit"
	stats_pb "github.com/checkpoint-restore/go-criu/v8/crit/images/stats"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/xlab/treeprint"
)

func RenderTreeView(tasks []Task) error {
	for _, task := range tasks {
		info, err := getCheckpointInfo(task)
		if err != nil {
			return err
		}

		tree := buildTree(info.containerInfo, info.configDump, info.archiveSizes)

		checkpointDirectory := filepath.Join(task.OutputDir, metadata.CheckpointDirectory)
		if Stats {
			dumpStats, err := crit.GetDumpStats(task.OutputDir)
			if err != nil {
				return fmt.Errorf("failed to get dump statistics: %w", err)
			}

			addDumpStatsToTree(tree, dumpStats)
		}

		if Metadata {
			addPodInfoToTree(tree, info)
		}

		if PsTree {
			c := crit.New(nil, nil, checkpointDirectory, false, false)
			psTree, err := c.ExplorePs()
			if err != nil {
				return fmt.Errorf("failed to get process tree: %w", err)
			}

			if PsTreeCmd {
				if err := updatePsTreeCommToCmdline(task.OutputDir, psTree); err != nil {
					return fmt.Errorf("failed to process command line arguments: %w", err)
				}
			}

			fds, err := func() ([]*crit.Fd, error) {
				if !Files {
					return nil, nil
				}
				c := crit.New(nil, nil, checkpointDirectory, false, false)
				fds, err := c.ExploreFds()
				if err != nil {
					return nil, fmt.Errorf("failed to get file descriptors: %w", err)
				}
				return fds, nil
			}()
			if err != nil {
				return err
			}

			sks, err := func() ([]*crit.Sk, error) {
				if !Sockets {
					return nil, nil
				}
				c := crit.New(nil, nil, checkpointDirectory, false, false)
				sks, err := c.ExploreSk()
				if err != nil {
					return nil, fmt.Errorf("failed to get sockets: %w", err)
				}
				return sks, nil
			}()
			if err != nil {
				return err
			}

			if err = addPsTreeToTree(tree, psTree, fds, sks, task.OutputDir); err != nil {
				return fmt.Errorf("failed to get process tree: %w", err)
			}
		}

		if Mounts {
			addMountsToTree(tree, info.specDump)
		}

		fmt.Printf("\nDisplaying container checkpoint tree view from %s\n\n", task.CheckpointFilePath)
		fmt.Println(tree.String())
	}

	return nil
}

func buildTree(ci *containerInfo, containerConfig *metadata.ContainerConfig, archiveSizes *archiveSizes) treeprint.Tree {
	if ci.Name == "" {
		ci.Name = "Container"
	}
	tree := treeprint.NewWithRoot(ci.Name)

	tree.AddBranch(fmt.Sprintf("Image: %s", containerConfig.RootfsImageName))
	tree.AddBranch(fmt.Sprintf("ID: %s", containerConfig.ID))
	tree.AddBranch(fmt.Sprintf("Runtime: %s", containerConfig.OCIRuntime))
	tree.AddBranch(fmt.Sprintf("Created: %s", ci.Created))
	if !containerConfig.CheckpointedAt.IsZero() {
		tree.AddBranch(fmt.Sprintf("Checkpointed: %s", containerConfig.CheckpointedAt.Format(time.RFC3339)))
	}
	tree.AddBranch(fmt.Sprintf("Engine: %s", ci.Engine))

	if ci.IP != "" {
		tree.AddBranch(fmt.Sprintf("IP: %s", ci.IP))
	}
	if ci.MAC != "" {
		tree.AddBranch(fmt.Sprintf("MAC: %s", ci.MAC))
	}

	checkpointSize := tree.AddBranch(fmt.Sprintf("Checkpoint size: %s", metadata.ByteToString(archiveSizes.checkpointSize)))
	if archiveSizes.pagesSize != 0 {
		checkpointSize.AddNode(fmt.Sprintf("Memory pages size: %s", metadata.ByteToString(archiveSizes.pagesSize)))
	}
	if archiveSizes.amdgpuPagesSize != 0 {
		checkpointSize.AddNode(fmt.Sprintf("AMD GPU memory pages size: %s", metadata.ByteToString(archiveSizes.amdgpuPagesSize)))
	}

	if archiveSizes.rootFsDiffTarSize != 0 {
		tree.AddBranch(fmt.Sprintf("Root FS diff size: %s", metadata.ByteToString(archiveSizes.rootFsDiffTarSize)))
	}

	return tree
}

func addMountsToTree(tree treeprint.Tree, specDump *spec.Spec) {
	mountsTree := tree.AddBranch("Overview of mounts")
	for _, data := range specDump.Mounts {
		mountTree := mountsTree.AddBranch(fmt.Sprintf("Destination: %s", data.Destination))
		mountTree.AddBranch(fmt.Sprintf("Type: %s", data.Type))
		mountTree.AddBranch(fmt.Sprintf("Source: %s", func() string {
			return data.Source
		}()))
	}
}

func addDumpStatsToTree(tree treeprint.Tree, dumpStats *stats_pb.DumpStatsEntry) {
	statsTree := tree.AddBranch("CRIU dump statistics")
	statsTree.AddBranch(fmt.Sprintf("Freezing time: %s", FormatTime(dumpStats.GetFreezingTime())))
	statsTree.AddBranch(fmt.Sprintf("Frozen time: %s", FormatTime(dumpStats.GetFrozenTime())))
	statsTree.AddBranch(fmt.Sprintf("Memdump time: %s", FormatTime(dumpStats.GetMemdumpTime())))
	statsTree.AddBranch(fmt.Sprintf("Memwrite time: %s", FormatTime(dumpStats.GetMemwriteTime())))
	statsTree.AddBranch(fmt.Sprintf("Pages scanned: %d", dumpStats.GetPagesScanned()))
	statsTree.AddBranch(fmt.Sprintf("Pages written: %d", dumpStats.GetPagesWritten()))
}

func addPsTreeToTree(
	tree treeprint.Tree,
	psTree *crit.PsTree,
	fds []*crit.Fd,
	sks []*crit.Sk,
	checkpointOutputDir string,
) error {
	psRoot := psTree
	if PID != 0 {
		ps := psTree.FindPs(PID)
		if ps == nil {
			return fmt.Errorf("no process with PID %d (use `inspect --ps-tree` to view all PIDs)", PID)
		}
		psRoot = ps
	}

	// processNodes is a recursive function to create
	// a new branch for each process and add its child
	// processes as child nodes of the branch.
	var processNodes func(treeprint.Tree, *crit.PsTree) error
	processNodes = func(tree treeprint.Tree, root *crit.PsTree) error {
		var metaBranchTag string

		taskState := crit.TaskState(root.Core.GetTc().GetTaskState())
		if taskState.IsAlive() {
			metaBranchTag = fmt.Sprintf("%d", root.PID)
		} else {
			metaBranchTag = fmt.Sprintf("%d (%s)", root.PID, taskState.String())
		}

		node := tree.AddMetaBranch(metaBranchTag, root.Comm)

		// Skip dead or zombie processes as they do not have other state, and
		// their children are inherited by init or the nearest subreaper process.
		if !taskState.IsAliveOrStopped() {
			return nil
		}

		// attach environment variables to process
		if PsTreeEnv {
			envVars, err := getPsEnvVars(checkpointOutputDir, root.PID)
			if err != nil {
				return err
			}

			if len(envVars) > 0 {
				nodeSubtree := node.AddBranch("Environment variables")
				for _, env := range envVars {
					nodeSubtree.AddBranch(env)
				}
			}
		}

		if err := showFiles(fds, node, root); err != nil {
			return err
		}

		if err := showSockets(sks, node, root); err != nil {
			return err
		}

		for _, child := range root.Children {
			if err := processNodes(node, child); err != nil {
				return err
			}
		}
		return nil
	}
	psTreeNode := tree.AddBranch("Process tree")

	return processNodes(psTreeNode, psRoot)
}

func showFiles(fds []*crit.Fd, node treeprint.Tree, root *crit.PsTree) error {
	if !Files {
		return nil
	}
	if fds == nil {
		return fmt.Errorf("failed to get file descriptors")
	}
	for _, fd := range fds {
		var nodeSubtree treeprint.Tree
		if fd.PId != root.PID {
			continue
		} else {
			nodeSubtree = node.AddBranch("Open files")
		}
		for _, file := range fd.Files {
			nodeSubtree.AddMetaBranch(strings.TrimSpace(file.Type+" "+file.Fd), file.Path)
		}
	}
	return nil
}

func showSockets(sks []*crit.Sk, node treeprint.Tree, root *crit.PsTree) error {
	if !Sockets {
		return nil
	}
	if sks == nil {
		return fmt.Errorf("failed to get sockets")
	}
	for _, sk := range sks {
		var nodeSubtree treeprint.Tree
		if sk.PId != root.PID {
			continue
		} else {
			nodeSubtree = node.AddBranch("Open sockets")
		}
		var data string
		var protocol string
		for _, socket := range sk.Sockets {
			protocol = socket.Protocol
			switch socket.FdType {
			case "UNIXSK":
				// UNIX sockets do not have a protocol assigned.
				// Hence, the meta value for the socket is just
				// the socket type.
				protocol = fmt.Sprintf("UNIX (%s)", socket.Type)
				data = socket.SrcAddr
				if len(data) == 0 {
					// Use an abstract socket address
					data = "@"
				}
			case "INETSK":
				if protocol == "TCP" {
					protocol = fmt.Sprintf("%s (%s)", socket.Protocol, socket.State)
				}
				data = fmt.Sprintf(
					"%s:%d -> %s:%d (↑ %s ↓ %s)",
					socket.SrcAddr, socket.SrcPort,
					socket.DestAddr, socket.DestPort,
					socket.SendBuf, socket.RecvBuf,
				)
			case "PACKETSK":
				data = fmt.Sprintf("↑ %s ↓ %s", socket.SendBuf, socket.RecvBuf)
			case "NETLINKSK":
				data = fmt.Sprintf("↑ %s ↓ %s", socket.SendBuf, socket.RecvBuf)
			}

			nodeSubtree.AddMetaBranch(protocol, data)
		}
	}
	return nil
}

// Recursively updates the Comm field of the psTree struct with the command line arguments
// from process memory pages
func updatePsTreeCommToCmdline(checkpointOutputDir string, psTree *crit.PsTree) error {
	taskState := crit.TaskState(psTree.Core.GetTc().GetTaskState())
	if !taskState.IsAliveOrStopped() {
		return nil
	}

	cmdline, err := getCmdline(checkpointOutputDir, psTree.PID)
	if err != nil {
		return err
	}
	if cmdline != "" {
		psTree.Comm = cmdline
	}

	for _, child := range psTree.Children {
		if err := updatePsTreeCommToCmdline(checkpointOutputDir, child); err != nil {
			return err
		}
	}
	return nil
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

func addPodInfoToTree(tree treeprint.Tree, info *checkpointInfo) {
	podTree := tree.AddBranch("Metadata")
	if len(info.containerInfo.Pod) > 0 {
		podTree.AddBranch(fmt.Sprintf("Pod name: %s", info.containerInfo.Pod))
	}
	if len(info.containerInfo.Namespace) > 0 {
		podTree.AddBranch(fmt.Sprintf("Kubernetes namespace: %s", info.containerInfo.Namespace))
	}
	if len(info.specDump.Annotations) > 0 {
		annotationTree := podTree.AddBranch("Annotations")
		for key := range info.specDump.Annotations {
			switch key {
			case "io.kubernetes.cri-o.Labels",
				"io.kubernetes.cri-o.Annotations",
				"io.kubernetes.cri-o.Metadata",
				"kubectl.kubernetes.io/last-applied-configuration":
				// We know that some annotations contain a JSON string we can pretty print
				local := make(map[string]interface{})
				if err := json.Unmarshal([]byte(info.specDump.Annotations[key]), &local); err != nil {
					continue
				}
				localTree := annotationTree.AddBranch(key)
				// Recursively add array/map JSON fields to the tree
				addMapToTree(localTree, local)
			case "io.kubernetes.cri-o.Volumes":
				// We know that some annotations contain a JSON string we can pretty print
				var local []mountAnnotations
				if err := json.Unmarshal([]byte(info.specDump.Annotations[key]), &local); err != nil {
					fmt.Printf("error: %s", err)
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
				annotationTree.AddBranch(fmt.Sprintf("%s: %s", key, info.specDump.Annotations[key]))
			}
		}
	}
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
