// SPDX-License-Identifier: Apache-2.0

// This file is used to handle memory pages analysis of container checkpoints

package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/checkpoint-restore/checkpointctl/internal"
	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v7/crit"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// chunkSize represents the default size of memory chunk (in bytes)
// to read for each output line when printing memory pages content in hexdump-like format.
const chunkSize = 16

var pageSize = os.Getpagesize()

func MemParse() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memparse",
		Short: "Analyze container checkpoint memory",
		RunE:  memparse,
		Args:  cobra.MinimumNArgs(1),
	}

	flags := cmd.Flags()

	flags.Uint32VarP(
		pID,
		"pid",
		"p",
		0,
		"Specify the PID of a process to analyze",
	)
	flags.StringVarP(
		outputFilePath,
		"output",
		"o",
		"",
		"Specify the output file to be written to",
	)

	return cmd
}

func memparse(cmd *cobra.Command, args []string) error {
	requiredFiles := []string{
		metadata.SpecDumpFile, metadata.ConfigDumpFile,
		filepath.Join(metadata.CheckpointDirectory, "pstree.img"),
		filepath.Join(metadata.CheckpointDirectory, "core-"),
	}

	if *pID == 0 {
		requiredFiles = append(
			requiredFiles,
			filepath.Join(metadata.CheckpointDirectory, "pagemap-"),
			filepath.Join(metadata.CheckpointDirectory, "mm-"),
		)
	} else {
		requiredFiles = append(
			requiredFiles,
			filepath.Join(metadata.CheckpointDirectory, fmt.Sprintf("pagemap-%d.img", *pID)),
			filepath.Join(metadata.CheckpointDirectory, fmt.Sprintf("mm-%d.img", *pID)),
		)
	}

	tasks, err := internal.CreateTasks(args, requiredFiles)
	if err != nil {
		return err
	}
	defer internal.CleanupTasks(tasks)

	if *pID != 0 {
		return printProcessMemoryPages(tasks[0])
	}

	return showProcessMemorySizeTables(tasks)
}

// Display processes memory sizes within the given container checkpoints.
func showProcessMemorySizeTables(tasks []internal.Task) error {
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

		c := crit.New(nil, nil, filepath.Join(task.OutputDir, "checkpoint"), false, false)
		psTree, err := c.ExplorePs()
		if err != nil {
			return fmt.Errorf("failed to get process tree: %w", err)
		}

		// Populate the table rows
		if err := traverseTree(psTree, task.OutputDir); err != nil {
			return err
		}

		fmt.Printf("\nDisplaying processes memory sizes from %s\n\n", task.CheckpointFilePath)
		table.Render()
	}

	return nil
}

func printProcessMemoryPages(task internal.Task) error {
	c := crit.New(nil, nil, filepath.Join(task.OutputDir, metadata.CheckpointDirectory), false, false)
	psTree, err := c.ExplorePs()
	if err != nil {
		return fmt.Errorf("failed to get process tree: %w", err)
	}

	// Check if PID exist within the checkpoint
	if *pID != 0 {
		ps := psTree.FindPs(*pID)
		if ps == nil {
			return fmt.Errorf("no process with PID %d (use `inspect --ps-tree` to view all PIDs)", *pID)
		}
	}

	memReader, err := crit.NewMemoryReader(
		filepath.Join(task.OutputDir, metadata.CheckpointDirectory),
		*pID, pageSize,
	)
	if err != nil {
		return err
	}

	// Unpack pages-[pagesID].img file for the given PID
	if err := internal.UntarFiles(
		task.CheckpointFilePath, task.OutputDir,
		[]string{filepath.Join(metadata.CheckpointDirectory, fmt.Sprintf("pages-%d.img", memReader.GetPagesID()))},
	); err != nil {
		return err
	}

	// Write the output to stdout by default
	var output io.Writer = os.Stdout
	var compact bool

	if *outputFilePath != "" {
		// Write output to file if --output is specified
		f, err := os.Create(*outputFilePath)
		if err != nil {
			return err
		}
		defer f.Close()
		output = f
		fmt.Printf("\nWriting memory pages content for process ID %d from checkpoint: %s to file: %s...\n",
			*pID, task.CheckpointFilePath, *outputFilePath,
		)
	} else {
		compact = true // Use a compact format when writing the output to stdout
		fmt.Printf("\nDisplaying memory pages content for process ID %d from checkpoint: %s\n\n", *pID, task.CheckpointFilePath)
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
