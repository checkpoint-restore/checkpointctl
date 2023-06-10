// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"os"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/spf13/cobra"
)

var (
	name      string
	version   string
	stats     bool
	mounts    bool
	fullPaths bool
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
	flags.BoolVarP(
		&stats,
		"stats",
		"s",
		false,
		"Print checkpointing statistics if available",
	)
	flags.BoolVarP(
		&mounts,
		"mounts",
		"m",
		false,
		"Print overview about mounts used in the checkpoints",
	)
	flags.BoolVarP(
		&fullPaths,
		"full-paths",
		"F",
		false,
		"Display mounts with full paths",
	)
	flags.BoolVarP(
		&showAll,
		"all",
		"A",
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
	}
	if fullPaths && !mounts {
		return fmt.Errorf("Cannot use --full-paths without --mounts/--all option")
	}

	input := args[0]
	tar, err := os.Stat(input)
	if err != nil {
		return err
	}
	if !tar.Mode().IsRegular() {
		return fmt.Errorf("input %s not a regular file", input)
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

	if err := untarFiles(input, dir, metadata.SpecDumpFile, metadata.ConfigDumpFile); err != nil {
		return err
	}

	return showContainerCheckpoint(dir, input)
}
