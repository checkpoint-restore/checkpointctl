package internal

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
)

func FormatTime(microseconds uint32) string {
	if microseconds < 1000 {
		return fmt.Sprintf("%d µs", microseconds)
	}

	var value float64
	var unit string

	if microseconds < 1000000 {
		value = float64(microseconds) / 1000
		unit = "ms"
	} else {
		duration := time.Duration(microseconds) * time.Microsecond
		value = duration.Seconds()
		unit = "s"
	}

	// Trim trailing zeros and dot
	formatted := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.5g", value), "0"), ".")

	return fmt.Sprintf("%s %s", formatted, unit)
}

type Task struct {
	CheckpointFilePath string
	OutputDir          string
}

func CreateTasks(args []string, requiredFiles []string) ([]Task, error) {
	tasks := make([]Task, 0, len(args))

	for _, input := range args {
		tar, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		if !tar.Mode().IsRegular() {
			return nil, fmt.Errorf("input %s not a regular file", input)
		}

		// Check if there is a checkpoint directory in the archive file
		checkpointDirExists, err := isFileInArchive(input, metadata.CheckpointDirectory, true)
		if err != nil {
			return nil, err
		}

		if !checkpointDirExists {
			return nil, fmt.Errorf("checkpoint directory is missing in the archive file: %s", input)
		}

		dir, err := os.MkdirTemp("", "checkpointctl")
		if err != nil {
			return nil, err
		}

		if err := UntarFiles(input, dir, requiredFiles); err != nil {
			return nil, err
		}

		tasks = append(tasks, Task{CheckpointFilePath: input, OutputDir: dir})
	}

	return tasks, nil
}

// cleanupTasks removes all output directories of given tasks
func CleanupTasks(tasks []Task) {
	for _, task := range tasks {
		if err := os.RemoveAll(task.OutputDir); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

// Constant values are based on kubectl's tabwriter settings:
// https://github.com/kubernetes/cli-runtime/blob/master/pkg/printers/tabwriter.go
const (
	tabwriterMinWidth = 6
	tabwriterWidth    = 4
	tabwriterPadding  = 3
	tabwriterPadChar  = ' '
	tabwriterFlags    = 0
)

// GetNewTabWriter returns a tabwriter that translates tabbed columns in input into properly aligned text.
func GetNewTabWriter(output io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(output, tabwriterMinWidth, tabwriterWidth, tabwriterPadding, tabwriterPadChar, tabwriterFlags)
}

// WriteTableHeader writes the header row and separator line for a table
func WriteTableHeader(w *tabwriter.Writer, header []string) {
	// Print header
	for i, h := range header {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, strings.ToUpper(h))
	}
	fmt.Fprintln(w)

	// Print separator line
	for i := range header {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, strings.Repeat("-", len(header[i])))
	}
	fmt.Fprintln(w)
}

// WriteTableRows writes the data rows for a table
func WriteTableRows(w *tabwriter.Writer, rows [][]string) {
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, cell)
		}
		fmt.Fprintln(w)
	}
}
