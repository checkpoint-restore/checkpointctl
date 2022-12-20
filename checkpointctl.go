// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/containers/storage/pkg/archive"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	name       string
	printStats bool
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
		&printStats,
		"print-stats",
		false,
		"Print checkpointing statistics if available",
	)

	return cmd
}

func show(cmd *cobra.Command, args []string) error {
	input := args[0]
	tar, err := os.Stat(input)
	if err != nil {
		return err
	}
	if !tar.Mode().IsRegular() {
		return errors.Wrapf(err, "input %s not a regular file", input)
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
		return errors.Wrapf(err, "unpacking of checkpoint archive %s failed", input)
	}

	return showContainerCheckpoint(dir)
}
