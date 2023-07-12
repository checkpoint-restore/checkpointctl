// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/spf13/cobra"
)

var (
	name      string
	version   string
	format    string
	stats     bool
	mounts    bool
	pID       uint32
	psTree    bool
	psTreeCmd bool
	files     bool
	showAll   bool
)

func main() {
	rootCommand := &cobra.Command{
		Use:   name,
		Short: name + " is a tool to read and manipulate checkpoint archives",
		Long: name + " is a tool to read and manipulate checkpoint archives as " +
			"created by Podman, CRI-O and containerd",
		SilenceUsage: true,
	}

	showCommand := setupShow()
	rootCommand.AddCommand(showCommand)

	inspectCommand := setupInspect()
	rootCommand.AddCommand(inspectCommand)

	rootCommand.Version = version

	if err := rootCommand.Execute(); err != nil {
		os.Exit(1)
	}
}

func setupShow() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "show",
		Short:                 "Show an overview of container checkpoints",
		RunE:                  show,
		Args:                  cobra.MinimumNArgs(1),
		DisableFlagsInUseLine: true,
	}

	return cmd
}

func show(cmd *cobra.Command, args []string) error {
	// Only "spec.dump" and "config.dump" are need when for the show sub-command
	requiredFiles := []string{metadata.SpecDumpFile, metadata.ConfigDumpFile}
	tasks, err := createTasks(args, requiredFiles)
	if err != nil {
		return err
	}
	defer cleanupTasks(tasks)

	return showContainerCheckpoints(tasks)
}

func setupInspect() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Display low-level information about a container checkpoint",
		RunE:  inspect,
		Args:  cobra.MinimumNArgs(1),
	}
	flags := cmd.Flags()

	flags.BoolVar(
		&stats,
		"stats",
		false,
		"Display checkpoint statistics",
	)
	flags.BoolVar(
		&mounts,
		"mounts",
		false,
		"Display an overview of mounts used in the container checkpoint",
	)
	flags.Uint32VarP(
		&pID,
		"pid",
		"p",
		0,
		"Display the process tree of a specific PID",
	)
	flags.BoolVar(
		&psTree,
		"ps-tree",
		false,
		"Display an overview of processes in the container checkpoint",
	)
	flags.BoolVar(
		&psTreeCmd,
		"ps-tree-cmd",
		false,
		"Display an overview of processes in the container checkpoint with full command line arguments",
	)
	flags.BoolVar(
		&files,
		"files",
		false,
		"Display the open file descriptors for processes in the container checkpoint",
	)
	flags.BoolVar(
		&showAll,
		"all",
		false,
		"Show all information about container checkpoints",
	)
	flags.StringVar(
		&format,
		"format",
		"tree",
		"Specify the output format: tree or json",
	)

	return cmd
}

func inspect(cmd *cobra.Command, args []string) error {
	if showAll {
		stats = true
		mounts = true
		psTree = true
		files = true
	}

	requiredFiles := []string{metadata.SpecDumpFile, metadata.ConfigDumpFile}

	if stats {
		requiredFiles = append(requiredFiles, "stats-dump")
	}

	if pID != 0 {
		// Enable displaying process tree if the PID filter is passed.
		psTree = true
	}

	if files {
		// Enable displaying process tree, even if it is not passed.
		// This is necessary to attach the files to the processes
		// that opened them and display this in the tree.
		psTree = true
		requiredFiles = append(
			requiredFiles,
			filepath.Join(metadata.CheckpointDirectory, "files.img"),
			// fs-*.img
			filepath.Join(metadata.CheckpointDirectory, "fs-"),
			// ids-*.img
			filepath.Join(metadata.CheckpointDirectory, "ids-"),
			// fdinfo-*.img
			filepath.Join(metadata.CheckpointDirectory, "fdinfo-"),
		)
	}

	if psTreeCmd {
		// Enable displaying process tree when using --ps-tree-cmd.
		psTree = true
		requiredFiles = append(
			requiredFiles,
			// Unpack pagemap-*.img, pages-*.img, and mm-*.img
			filepath.Join(metadata.CheckpointDirectory, "pagemap-"),
			filepath.Join(metadata.CheckpointDirectory, "pages-"),
			filepath.Join(metadata.CheckpointDirectory, "mm-"),
		)
	}

	if psTree {
		requiredFiles = append(
			requiredFiles,
			filepath.Join(metadata.CheckpointDirectory, "pstree.img"),
			// core-*.img
			filepath.Join(metadata.CheckpointDirectory, "core-"),
		)
	}

	tasks, err := createTasks(args, requiredFiles)
	if err != nil {
		return err
	}
	defer cleanupTasks(tasks)

	switch format {
	case "tree":
		return renderTreeView(tasks)
	case "json":
		return fmt.Errorf("json format is not supported yet")
	default:
		return fmt.Errorf("invalid output format: %s", format)
	}
}

type task struct {
	checkpointFilePath string
	outputDir          string
}

func createTasks(args []string, requiredFiles []string) ([]task, error) {
	tasks := make([]task, 0, len(args))

	for _, input := range args {
		tar, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		if !tar.Mode().IsRegular() {
			return nil, fmt.Errorf("input %s not a regular file", input)
		}

		// Check if there is a checkpoint directory in the archive file
		checkpointDirExists, err := isFileInArchive(input, metadata.CheckpointDirectory, true)
		if err != nil {
			return nil, err
		}

		if !checkpointDirExists {
			return nil, fmt.Errorf("checkpoint directory is missing in the archive file: %s", input)
		}

		dir, err := os.MkdirTemp("", "checkpointctl")
		if err != nil {
			return nil, err
		}

		if err := untarFiles(input, dir, requiredFiles); err != nil {
			return nil, err
		}

		tasks = append(tasks, task{checkpointFilePath: input, outputDir: dir})
	}

	return tasks, nil
}

// cleanupTasks removes all output directories of given tasks
func cleanupTasks(tasks []task) {
	for _, task := range tasks {
		if err := os.RemoveAll(task.outputDir); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
