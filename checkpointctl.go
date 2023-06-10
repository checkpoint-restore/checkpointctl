// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/spf13/cobra"
)

var (
	name      string
	version   string
	stats     bool
	mounts    bool
	fullPaths bool
	psTree    bool
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
	rootCommand.Version = version

	if err := rootCommand.Execute(); err != nil {
		os.Exit(1)
	}
}

func setupShow() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show information about available checkpoints",
		RunE:  show,
		Args:  cobra.MinimumNArgs(1),
	}
	flags := cmd.Flags()
	flags.BoolVar(
		&stats,
		"print-stats",
		false,
		"Print checkpointing statistics if available",
	)
	flags.BoolVar(
		&stats,
		"stats",
		false,
		"Print checkpointing statistics if available",
	)
	flags.BoolVar(
		&mounts,
		"mounts",
		false,
		"Print overview about mounts used in the checkpoints",
	)
	flags.BoolVar(
		&psTree,
		"ps-tree",
		false,
		"Print overview about the process tree in the checkpoints",
	)
	flags.BoolVar(
		&fullPaths,
		"full-paths",
		false,
		"Display mounts with full paths",
	)
	flags.BoolVar(
		&showAll,
		"all",
		false,
		"Display all additional information about the checkpoints",
	)

	err := flags.MarkHidden("print-stats")
	if err != nil {
		log.Fatal(err)
	}
	return cmd
}

func show(cmd *cobra.Command, args []string) error {
	if showAll {
		stats = true
		mounts = true
		psTree = true
	}
	if fullPaths && !mounts {
		return fmt.Errorf("Cannot use --full-paths without --mounts/--all option")
	}

	tasks := make([]task, 0, len(args))

	for _, input := range args {
		tar, err := os.Stat(input)
		if err != nil {
			return err
		}
		if !tar.Mode().IsRegular() {
			return fmt.Errorf("input %s not a regular file", input)
		}

		// A list of files that need to be unarchived. The files need not be
		// full paths. Even a substring of the file name is valid.
		files := []string{metadata.SpecDumpFile, metadata.ConfigDumpFile}

		if stats {
			files = append(files, "stats-dump")
		}

		if psTree {
			files = append(
				files,
				filepath.Join(metadata.CheckpointDirectory, "pstree.img"),
				// All core-*.img files
				filepath.Join(metadata.CheckpointDirectory, "core-"),
			)
		}

		// Check if there is a checkpoint directory in the archive file
		checkpointDirExists, err := isFileInArchive(input, metadata.CheckpointDirectory, true)
		if err != nil {
			return err
		}

		if !checkpointDirExists {
			return fmt.Errorf("checkpoint directory is missing in the archive file: %s", input)
		}

		dir, err := os.MkdirTemp("", "checkpointctl")
		if err != nil {
			return err
		}
		defer func() {
			if err := os.RemoveAll(dir); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}()

		if err := untarFiles(input, dir, files); err != nil {
			return err
		}

		tasks = append(tasks, task{checkpointFilePath: input, outputDir: dir})
	}

	return showContainerCheckpoints(tasks)
}

type task struct {
	checkpointFilePath string
	outputDir          string
}
