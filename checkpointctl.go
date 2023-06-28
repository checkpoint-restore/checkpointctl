// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/spf13/cobra"
)

var (
	name    string
	version string
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
		Use:                   "show",
		Short:                 "Show information about available checkpoints",
		RunE:                  show,
		Args:                  cobra.MinimumNArgs(1),
		DisableFlagsInUseLine: true,
	}

	return cmd
}

func show(cmd *cobra.Command, args []string) error {
	tasks := make([]task, 0, len(args))

	for _, input := range args {
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

		// A list of files that need to be unarchived. The files need not be
		// full paths. Even a substring of the file name is valid.
		files := []string{metadata.SpecDumpFile, metadata.ConfigDumpFile}
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
