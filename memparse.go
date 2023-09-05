// SPDX-License-Identifier: Apache-2.0

// This file is used to handle memory pages analysis of container checkpoints

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v6/crit"
	"github.com/olekukonko/tablewriter"
)

// chunkSize represents the default size of memory chunk (in bytes)
// to read for each output line when printing memory pages content in hexdump-like format.
const chunkSize = 16

// Display processes memory sizes within the given container checkpoints.
func showProcessMemorySizeTables(tasks []task) error {
	// Initialize the table
	table := tablewriter.NewWriter(os.Stdout)
	header := []string{
		"PID",
		"Process name",
		"Memory size",
		"Shared memory size",
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

		shmemSize, err := memReader.GetShmemSize()
		if err != nil {
			return err
		}

		table.Append([]string{
			fmt.Sprintf("%d", root.PID),
			root.Comm,
			metadata.ByteToString(memSize),
			metadata.ByteToString(shmemSize),
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

func printProcessMemoryPages(task task) error {
	c := crit.New(nil, nil, filepath.Join(task.outputDir, metadata.CheckpointDirectory), false, false)
	psTree, err := c.ExplorePs()
	if err != nil {
		return fmt.Errorf("failed to get process tree: %w", err)
	}

	// Check if PID exist within the checkpoint
	if pID != 0 {
		ps := psTree.FindPs(pID)
		if ps == nil {
			return fmt.Errorf("no process with PID %d (use `inspect --ps-tree` to view all PIDs)", pID)
		}
	}

	memReader, err := crit.NewMemoryReader(
		filepath.Join(task.outputDir, metadata.CheckpointDirectory),
		pID, pageSize,
	)
	if err != nil {
		return err
	}

	// Unpack pages-[pagesID].img file for the given PID
	if err := untarFiles(
		task.checkpointFilePath, task.outputDir,
		[]string{filepath.Join(metadata.CheckpointDirectory, fmt.Sprintf("pages-%d.img", memReader.GetPagesID()))},
	); err != nil {
		return err
	}

	// Write the output to stdout by default
	var output io.Writer = os.Stdout
	var compact bool

	if outputFilePath != "" {
		// Write output to file if --output is specified
		f, err := os.Create(outputFilePath)
		if err != nil {
			return err
		}
		defer f.Close()
		output = f
		fmt.Printf("\nWriting memory pages content for process ID %d from checkpoint: %s to file: %s...\n",
			pID, task.checkpointFilePath, outputFilePath,
		)
	} else {
		compact = true // Use a compact format when writing the output to stdout
		fmt.Printf("\nDisplaying memory pages content for process ID %d from checkpoint: %s\n\n", pID, task.checkpointFilePath)
	}

	fmt.Fprintln(output, "Address           Hexadecimal                                       ASCII            ")
	fmt.Fprintln(output, "-------------------------------------------------------------------------------------")

	pagemapEntries := memReader.GetPagemapEntries()
	for _, entry := range pagemapEntries {
		start := entry.GetVaddr()
		end := start + (uint64(pageSize) * uint64(entry.GetNrPages()))
		buf, err := memReader.GetMemPages(start, end)
		if err != nil {
			return err
		}

		hexdump(output, buf, start, compact)
	}
	return nil
}

// hexdump generates a hexdump of the buffer 'buf' starting at the virtual address 'start'
// and writes the output to 'out'. If compact is true, consecutive duplicate rows will be represented
// with an asterisk (*).
func hexdump(out io.Writer, buf *bytes.Buffer, vaddr uint64, compact bool) {
	var prevAscii string
	var isDuplicate bool
	for buf.Len() > 0 {
		row := buf.Next(chunkSize)
		hex, ascii := generateHexAndAscii(row)

		if compact {
			if prevAscii == ascii {
				if !isDuplicate {
					fmt.Fprint(out, "*\n")
				}
				isDuplicate = true
			} else {
				fmt.Fprintf(out, "%016x  %s |%s|\n", vaddr, hex, ascii)
				isDuplicate = false
			}
		} else {
			fmt.Fprintf(out, "%016x  %s |%s|\n", vaddr, hex, ascii)
		}

		vaddr += chunkSize
		prevAscii = ascii
	}
}

// generateHexAndAscii takes a byte slice and generates its hexadecimal and ASCII representations.
func generateHexAndAscii(data []byte) (string, string) {
	var hex, ascii string
	for i := 0; i < len(data); i++ {
		if data[i] < 32 || data[i] >= 127 {
			ascii += "."
			hex += fmt.Sprintf("%02x ", data[i])
		} else {
			ascii += string(data[i])
			hex += fmt.Sprintf("%02x ", data[i])
		}
	}

	return hex, ascii
}
