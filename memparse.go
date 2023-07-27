// SPDX-License-Identifier: Apache-2.0

// This file is used to handle memory pages analysis of container checkpoints

package main

import (
	"fmt"
	"os"
	"path/filepath"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v6/crit"
	"github.com/olekukonko/tablewriter"
)

// Display processes memory sizes within the given container checkpoints.
func showProcessMemorySizeTables(tasks []task) error {
	// Initialize the table
	table := tablewriter.NewWriter(os.Stdout)
	header := []string{
		"PID",
		"Process name",
		"Memory size",
	}
	table.SetHeader(header)
	table.SetAutoMergeCells(false)
	table.SetRowLine(true)

	// Function to recursively traverse the process tree and populate the table rows
	var traverseTree func(*crit.PsTree, string) error
	traverseTree = func(root *crit.PsTree, checkpointOutputDir string) error {
		memReader, err := crit.NewMemoryReader(
			filepath.Join(checkpointOutputDir, metadata.CheckpointDirectory),
			root.PID, pageSize,
		)
		if err != nil {
			return err
		}

		pagemapEntries := memReader.GetPagemapEntries()

		var memSize int64

		for _, entry := range pagemapEntries {
			memSize += int64(*entry.NrPages) * int64(pageSize)
		}

		table.Append([]string{
			fmt.Sprintf("%d", root.PID),
			root.Comm,
			metadata.ByteToString(memSize),
		})

		for _, child := range root.Children {
			if err := traverseTree(child, checkpointOutputDir); err != nil {
				return err
			}
		}
		return nil
	}

	for _, task := range tasks {
		// Clear the table before processing each checkpoint task
		table.ClearRows()

		c := crit.New(nil, nil, filepath.Join(task.outputDir, "checkpoint"), false, false)
		psTree, err := c.ExplorePs()
		if err != nil {
			return fmt.Errorf("failed to get process tree: %w", err)
		}

		// Populate the table rows
		if err := traverseTree(psTree, task.outputDir); err != nil {
			return err
		}

		fmt.Printf("\nDisplaying processes memory sizes from %s\n\n", task.checkpointFilePath)
		table.Render()
	}

	return nil
}
