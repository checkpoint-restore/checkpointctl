// SPDX-License-Identifier: Apache-2.0

// This file is used to handle memory pages analysis of container checkpoints

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"

	"github.com/checkpoint-restore/checkpointctl/internal"
	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v8/crit"
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

	flags.StringVarP(
		searchPattern,
		"search",
		"s",
		"",
		"Search for a string pattern in memory pages",
	)

	flags.StringVarP(
		searchRegexPattern,
		"search-regex",
		"r",
		"",
		"Search for a regex pattern in memory pages",
	)

	flags.IntVarP(
		searchContext,
		"context",
		"c",
		0,
		"Print the specified number of bytes surrounding each match",
	)

	flags.StringVar(
		addressRange,
		"address-range",
		"",
		"Filter memory by virtual address range, e.g. \"0x7f0000000000-0x7f0000010000\"",
	)

	return cmd
}

func memparse(cmd *cobra.Command, args []string) error {
	var (
		rangeStart uint64
		rangeEnd   uint64
		hasRange   bool
	)

	if cmd.Flags().Changed("address-range") {
		if *addressRange == "" {
			return fmt.Errorf("invalid address range: cannot be empty")
		}
		if *pID == 0 {
			return fmt.Errorf("--address-range requires --pid")
		}
		var err error
		rangeStart, rangeEnd, err = parseAddrRange(*addressRange)
		if err != nil {
			return err
		}
		hasRange = true
	}

	requiredFiles := []string{
		metadata.SpecDumpFile, metadata.ConfigDumpFile,
		filepath.Join(metadata.CheckpointDirectory, "inventory.img"),
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

	if *searchPattern != "" || *searchRegexPattern != "" {
		return printMemorySearchResultForPID(tasks[0], hasRange, rangeStart, rangeEnd)
	}

	if *pID != 0 {
		return printProcessMemoryPages(tasks[0], hasRange, rangeStart, rangeEnd)
	}

	return showProcessMemorySizeTables(tasks)
}

func printCRIUCompression(task internal.Task) error {
	checkpointDirectory := filepath.Join(task.OutputDir, metadata.CheckpointDirectory)
	compression, err := crit.ReadCompressionInfo(checkpointDirectory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to get CRIU compression information: %w", err)
	}

	// Older archives may omit inventory.img and contain uncompressed pages.
	state := "Disabled"
	if compression.Compressed() {
		state = "Enabled"
	}
	fmt.Printf("CRIU compression: %s\n\n", state)
	return nil
}

// Display processes memory sizes within the given container checkpoints.
func showProcessMemorySizeTables(tasks []internal.Task) error {
	header := []string{
		"PID",
		"Process name",
		"Memory size",
		"Shared memory size",
	}

	// Function to recursively traverse the process tree and populate the table rows
	var traverseTree func(*crit.PsTree, string, *[][]string) error
	traverseTree = func(root *crit.PsTree, checkpointOutputDir string, rows *[][]string) error {
		taskState := crit.TaskState(root.Core.GetTc().GetTaskState())

		// Ignore zombies and dead processes as they don't have memory pages.
		if !taskState.IsAliveOrStopped() {
			return nil
		}

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
			// Get the page count from the current format or the field used by
			// older checkpoints. This counts memory pages before compression.
			pages := crit.PagemapPageCount(entry)

			// Each page adds pageSize bytes to memSize. MaxInt64 is the largest
			// byte total we can store. Subtract the bytes already counted, then
			// divide by pageSize to find how many more pages fit. Check this limit
			// before multiplying or adding to avoid an incorrect total from overflow.
			if pages > uint64((math.MaxInt64-memSize)/int64(pageSize)) {
				return fmt.Errorf("memory size for PID %d exceeds supported size", root.PID)
			}

			memSize += int64(pages) * int64(pageSize)
		}

		shmemSize, err := memReader.GetShmemSize()
		if err != nil {
			return err
		}

		row := []string{
			fmt.Sprintf("%d", root.PID),
			root.Comm,
			metadata.ByteToString(memSize),
			metadata.ByteToString(shmemSize),
		}
		*rows = append(*rows, row)

		for _, child := range root.Children {
			if err := traverseTree(child, checkpointOutputDir, rows); err != nil {
				return err
			}
		}
		return nil
	}

	for _, task := range tasks {
		w := internal.GetNewTabWriter(os.Stdout)
		var rows [][]string

		c := crit.New(nil, nil, filepath.Join(task.OutputDir, "checkpoint"), false, false)
		psTree, err := c.ExplorePs()
		if err != nil {
			return fmt.Errorf("failed to get process tree: %w", err)
		}

		// Populate the table rows
		if err := traverseTree(psTree, task.OutputDir, &rows); err != nil {
			return err
		}

		fmt.Printf("\nDisplaying processes memory sizes from %s\n\n", task.CheckpointFilePath)
		if err := printCRIUCompression(task); err != nil {
			return err
		}

		internal.WriteTableHeader(w, header)
		internal.WriteTableRows(w, rows)

		w.Flush()
	}

	return nil
}

func printProcessMemoryPages(task internal.Task, hasRange bool, rangeStart, rangeEnd uint64) error {
	c := crit.New(nil, nil, filepath.Join(task.OutputDir, metadata.CheckpointDirectory), false, false)
	psTree, err := c.ExplorePs()
	if err != nil {
		return fmt.Errorf("failed to get process tree: %w", err)
	}

	// If PID=0, show all PIDs; otherwise, parse the specified PID's memory.
	if *pID != 0 {
		// Check if PID exist within the checkpoint
		ps := psTree.FindPs(*pID)
		if ps == nil {
			return fmt.Errorf("no process with PID %d (use `inspect --ps-tree` to view all PIDs)", *pID)
		}

		// Check if the specified process has memory pages
		taskState := crit.TaskState(ps.Core.GetTc().GetTaskState())
		if !taskState.IsAliveOrStopped() {
			return fmt.Errorf("process %d has no memory pages (task state is zombie or dead)", ps.PID)
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
	if err := printCRIUCompression(task); err != nil {
		return err
	}

	fmt.Fprintln(output, "Address           Hexadecimal                                       ASCII            ")
	fmt.Fprintln(output, "-------------------------------------------------------------------------------------")

	pagemapEntries := memReader.GetPagemapEntries()
	for _, entry := range pagemapEntries {
		start := entry.GetVaddr()
		pages := crit.PagemapPageCount(entry)

		// The end address is the start address plus the number of bytes.
		// Subtract start from the largest uint64 address, then divide by
		// pageSize to find how many pages fit. Check before multiplying or
		// adding so an overflow cannot wrap the end address back to a low value.
		if pages > (math.MaxUint64-start)/uint64(pageSize) {
			return fmt.Errorf("memory range at %#x overflows", start)
		}

		end := start + uint64(pageSize)*pages
		clipStart := start
		clipEnd := end
		if hasRange {
			clipStart = max(start, rangeStart)
			clipEnd = min(end, rangeEnd)
			if clipStart >= clipEnd {
				continue
			}
		}

		buf, err := memReader.GetMemPages(clipStart, clipEnd)
		if err != nil {
			return err
		}

		hexdump(output, buf, clipStart, compact)
	}
	return nil
}

// hexdump generates a hexdump of the buffer 'buf' starting at the virtual address 'start'
// and writes the output to 'out'. If compact is true, consecutive duplicate rows will be represented
// with an asterisk (*).
func hexdump(out io.Writer, buf *bytes.Buffer, vaddr uint64, compact bool) {
	var prevRow []byte
	var isDuplicate bool
	for buf.Len() > 0 {
		row := buf.Next(chunkSize)
		hex, ascii := generateHexAndAscii(row)

		if compact {
			if bytes.Equal(prevRow, row) {
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
		prevRow = append(prevRow[:0], row...)
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

// Searches for a pattern in the memory of a given PID and prints the results.
func printMemorySearchResultForPID(task internal.Task, hasRange bool, rangeStart, rangeEnd uint64) error {
	c := crit.New(nil, nil, filepath.Join(task.OutputDir, metadata.CheckpointDirectory), false, false)
	psTree, err := c.ExplorePs()
	if err != nil {
		return fmt.Errorf("failed to get process tree: %w", err)
	}

	// Check if PID exist within the checkpoint
	ps := psTree.FindPs(*pID)
	if ps == nil {
		return fmt.Errorf("no process with PID %d (use `inspect --ps-tree` to view all PIDs)", *pID)
	}

	// Check if the process has memory pages
	taskState := crit.TaskState(ps.Core.GetTc().GetTaskState())
	if !taskState.IsAliveOrStopped() {
		return fmt.Errorf("process %d has no memory pages (task state is zombie or dead)", ps.PID)
	}

	memReader, err := crit.NewMemoryReader(
		filepath.Join(task.OutputDir, metadata.CheckpointDirectory),
		*pID, pageSize,
	)
	if err != nil {
		return fmt.Errorf("failed to create memory reader: %w", err)
	}

	if err := internal.UntarFiles(
		task.CheckpointFilePath, task.OutputDir,
		[]string{filepath.Join(metadata.CheckpointDirectory, fmt.Sprintf("pages-%d.img", memReader.GetPagesID()))},
	); err != nil {
		return fmt.Errorf("failed to extract pages file: %w", err)
	}

	pattern := *searchPattern
	escapeRegExpCharacters := true
	if pattern == "" {
		pattern = *searchRegexPattern
		escapeRegExpCharacters = false
	}

	var results []crit.PatternMatch

	if hasRange {
		if *searchContext < 0 {
			return fmt.Errorf("context size cannot be negative")
		}

		var re *regexp.Regexp
		if escapeRegExpCharacters {
			re = regexp.MustCompile(regexp.QuoteMeta(pattern))
		} else {
			var err error
			re, err = regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("failed to compile regex pattern: %w", err)
			}
		}

		read := func(start, end uint64) ([]byte, error) {
			buf, err := memReader.GetMemPages(start, end)
			if err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		}

		for _, entry := range memReader.GetPagemapEntries() {
			start := entry.GetVaddr()
			pages := crit.PagemapPageCount(entry)
			if pages > (math.MaxUint64-start)/uint64(pageSize) {
				return fmt.Errorf("memory range at %#x overflows", start)
			}
			end := start + uint64(pageSize)*pages

			// Skip lazy pages with no stored image content.
			flags := crit.EffectivePagemapFlags(entry)
			if flags&(crit.PagemapPresent|crit.PagemapParent) == 0 && flags&crit.PagemapLazy != 0 {
				continue
			}

			clipStart, clipEnd, ok := clipRange(start, end, rangeStart, rangeEnd)
			if !ok {
				continue
			}

			matches, err := searchClipped(read, clipStart, clipEnd, re, *searchContext, rangeSearchChunk)
			if err != nil {
				return err
			}
			results = append(results, matches...)
		}
	} else {
		var err error
		results, err = memReader.SearchPattern(pattern, escapeRegExpCharacters, *searchContext, 0)
		if err != nil {
			return fmt.Errorf("failed to search pattern in memory: %w", err)
		}
	}
	if err := printCRIUCompression(task); err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Printf("No matches for pattern \"%s\" in the memory of PID %d\n", pattern, *pID)
		return nil
	}

	w := internal.GetNewTabWriter(os.Stdout)
	header := []string{"Address", "Match", "Instance"}

	internal.WriteTableHeader(w, header)

	// Build rows
	var rows [][]string
	for i, result := range results {
		row := []string{
			fmt.Sprintf("%016x", result.Vaddr),
			result.Match,
			fmt.Sprintf("%d", i+1),
		}
		rows = append(rows, row)
	}

	internal.WriteTableRows(w, rows)

	w.Flush()
	return nil
}
