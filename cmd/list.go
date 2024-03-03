// SPDX-License-Identifier: Apache-2.0

// This file is used to show the list of container checkpoints

package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/checkpoint-restore/checkpointctl/internal"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	defaultCheckpointPath     = "/var/lib/kubelet/checkpoints/"
	additionalCheckpointPaths []string
)

func List() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List checkpoints stored in the default and additional paths",
		RunE:  list,
	}

	flags := cmd.Flags()
	flags.StringSliceVarP(
		&additionalCheckpointPaths,
		"paths",
		"p",
		[]string{},
		"Specify additional paths to include in checkpoint listing",
	)

	return cmd
}

func list(cmd *cobra.Command, args []string) error {
	allPaths := append([]string{defaultCheckpointPath}, additionalCheckpointPaths...)
	showTable := false

	table := tablewriter.NewWriter(os.Stdout)
	header := []string{
		"Namespace",
		"Pod",
		"Container",
		"Engine",
		"Time Checkpointed",
		"Checkpoint Name",
	}

	table.SetHeader(header)
	table.SetAutoMergeCells(false)
	table.SetRowLine(true)

	for _, checkpointPath := range allPaths {
		files, err := filepath.Glob(filepath.Join(checkpointPath, "checkpoint-*"))
		if err != nil {
			return err
		}

		if len(files) == 0 {
			continue
		}

		showTable = true
		fmt.Printf("Listing checkpoints in path: %s\n", checkpointPath)

		for _, file := range files {
			chkptConfig, err := internal.ExtractConfigDump(file)
			if err != nil {
				log.Printf("Error extracting information from %s: %v\n", file, err)
				continue
			}

			row := []string{
				chkptConfig.Namespace,
				chkptConfig.Pod,
				chkptConfig.Container,
				chkptConfig.ContainerManager,
				chkptConfig.Timestamp.Format(time.RFC822),
				filepath.Base(file),
			}

			table.Append(row)
		}
	}

	if !showTable {
		fmt.Println("No checkpoints found")
		return nil
	}

	table.Render()
	return nil
}
