package main

import (
	"fmt"
	"path/filepath"
	"strings"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v6/crit"
	stats_pb "github.com/checkpoint-restore/go-criu/v6/crit/images/stats"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/xlab/treeprint"
)

func renderTreeView(tasks []task) error {
	for _, task := range tasks {
		info, err := getCheckpointInfo(task)
		if err != nil {
			return err
		}

		tree := buildTree(info.containerInfo, info.configDump, info.archiveSizes)

		if stats {
			dumpStats, err := crit.GetDumpStats(task.outputDir)
			if err != nil {
				return fmt.Errorf("failed to get dump statistics: %w", err)
			}

			addDumpStatsToTree(tree, dumpStats)
		}

		if psTree {
			c := crit.New(nil, nil, filepath.Join(task.outputDir, "checkpoint"), false, false)
			psTree, err := c.ExplorePs()
			if err != nil {
				return fmt.Errorf("failed to get process tree: %w", err)
			}

			addPsTreeToTree(tree, psTree)
		}

		if files {
			c := crit.New(nil, nil, filepath.Join(task.outputDir, "checkpoint"), false, false)
			fds, err := c.ExploreFds()
			if err != nil {
				return fmt.Errorf("failed to get file descriptors: %w", err)
			}

			addFdsToTree(tree, fds)
		}

		if mounts {
			addMountsToTree(tree, info.specDump)
		}

		fmt.Printf("\nDisplaying container checkpoint tree view from %s\n\n", task.checkpointFilePath)
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
	tree.AddBranch(fmt.Sprintf("Engine: %s", ci.Engine))

	if ci.IP != "" {
		tree.AddBranch(fmt.Sprintf("IP: %s", ci.IP))
	}
	if ci.MAC != "" {
		tree.AddBranch(fmt.Sprintf("MAC: %s", ci.MAC))
	}

	tree.AddBranch(fmt.Sprintf("Checkpoint size: %s", metadata.ByteToString(archiveSizes.checkpointSize)))

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
	statsTree.AddBranch(fmt.Sprintf("Freezing time: %d us", dumpStats.GetFreezingTime()))
	statsTree.AddBranch(fmt.Sprintf("Frozen time: %d us", dumpStats.GetFrozenTime()))
	statsTree.AddBranch(fmt.Sprintf("Memdump time: %d us", dumpStats.GetMemdumpTime()))
	statsTree.AddBranch(fmt.Sprintf("Memwrite time: %d us", dumpStats.GetMemwriteTime()))
	statsTree.AddBranch(fmt.Sprintf("Pages scanned: %d us", dumpStats.GetPagesScanned()))
	statsTree.AddBranch(fmt.Sprintf("Pages written: %d us", dumpStats.GetPagesWritten()))
}

func addPsTreeToTree(tree treeprint.Tree, psTree *crit.PsTree) {
	// processNodes is a recursive function to create
	// a new branch for each process and add its child
	// processes as child nodes of the branch.
	var processNodes func(treeprint.Tree, *crit.PsTree)
	processNodes = func(tree treeprint.Tree, root *crit.PsTree) {
		node := tree.AddMetaBranch(root.PID, root.Comm)
		for _, child := range root.Children {
			processNodes(node, child)
		}
	}
	psTreeNode := tree.AddBranch("Process tree")
	processNodes(psTreeNode, psTree)
}

func addFdsToTree(tree treeprint.Tree, fds []*crit.Fd) {
	var node treeprint.Tree
	for _, fd := range fds {
		node = tree.FindByMeta(fd.PId)
		for _, file := range fd.Files {
			node.AddMetaBranch(strings.TrimSpace(file.Type+" "+file.Fd), file.Path)
		}
	}
}
