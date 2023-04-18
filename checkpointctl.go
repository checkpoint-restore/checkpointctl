// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/containers/storage/pkg/archive"
	"github.com/spf13/cobra"
)

var (
	name       string
	version    string
	printStats bool
	showMounts bool
	fullPaths  bool
	showAll    bool
)

func main() {
	rootCommand := &cobra.Command{
		Use:   name,
		Short: name + " is a tool to read and manipulate checkpoint archives",
		Long: name + " is a tool to read and manipulate checkpoint archives as " +
			"created by Podman, CRI-O and containerd",
		SilenceUsage: true,
	}

	showCommand, err := setupShow()
	if err != nil {
		os.Exit(1)
	}
	
	rootCommand.AddCommand(showCommand)
	rootCommand.Version = version

	if err = rootCommand.Execute(); err != nil {
		os.Exit(1)
	}
}

func setupShow() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show information about available checkpoints",
		RunE:  show,
		Args:  cobra.MinimumNArgs(1),
	}
	flags := cmd.Flags()
	flags.BoolVar(
		&printStats,
		"print-stats",
		false,
		"Print checkpointing statistics if available",
	)
	flags.BoolVar(
		&printStats,
		"stats",
		false,
		"Print checkpointing statistics if available",
	)
	flags.BoolVar(
		&showMounts,
		"mounts",
		false,
		"Print overview about mounts used in the checkpoints",
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
	return cmd, err
}

func show(cmd *cobra.Command, args []string) error {
	if showAll {
		printStats = true
		showMounts = true
	}
	if fullPaths && !showMounts {
		return fmt.Errorf("Cannot use --full-paths without --mounts/-all option")
	}

	input := args[0]
	tar, err := os.Stat(input)
	if err != nil {
		return err
	}
	if !tar.Mode().IsRegular() {
		return fmt.Errorf("input %s not a regular file", input)
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

	if err := archive.UntarPath(input, dir); err != nil {
		return fmt.Errorf("unpacking of checkpoint archive %s failed: %w", input, err)
	}
	return showContainerCheckpoint(dir)
}
