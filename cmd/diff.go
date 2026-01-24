package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/checkpoint-restore/checkpointctl/internal"
	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/spf13/cobra"
)

// creates the command
func Diff() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <checkpointA> <checkpointB>",
		Short: "Show changes between two container checkpoints",
		Args:  cobra.ExactArgs(2),
		RunE:  diff,
	}

	flags := cmd.Flags()
	flags.StringVar(
		format,
		"format",
		"tree",
		"Specify output format: tree or json",
	)
	flags.BoolVar(
		psTreeCmd,
		"ps-tree-cmd",
		false,
		"Include full command lines in process tree diff",
	)
	flags.BoolVar(
		psTreeEnv,
		"ps-tree-env",
		false,
		"Include environment variables in process tree diff",
	)
	flags.BoolVar(
		files,
		"files",
		false,
		"Include file descriptors in the diff",
	)
	flags.BoolVar(
		sockets,
		"sockets",
		false,
		"Include sockets in the diff",
	)

	return cmd
}

// diff executes the checkpoint diff logic
func diff(cmd *cobra.Command, args []string) error {
	checkA := args[0]
	checkB := args[1]

	requiredFiles := []string{
		metadata.SpecDumpFile,
		metadata.ConfigDumpFile,
	}

	if *files || *sockets || *psTreeCmd || *psTreeEnv {
		// Include all files necessary for deep diffs
		for _, f := range []string{"files.img", "fs-", "ids-", "fdinfo-", "pagemap-", "pages-", "mm-", "pstree.img", "core-"} {
			requiredFiles = append(requiredFiles, filepath.Join(metadata.CheckpointDirectory, f))
		}
	}

	// Load tasks from both checkpoints
	tasksAVal, err := internal.CreateTasks([]string{checkA}, requiredFiles)
	if err != nil {
		return fmt.Errorf("failed to load checkpointA: %w", err)
	}
	defer internal.CleanupTasks(tasksAVal)

	tasksBVal, err := internal.CreateTasks([]string{checkB}, requiredFiles)
	if err != nil {
		return fmt.Errorf("failed to load checkpointB: %w", err)
	}
	defer internal.CleanupTasks(tasksBVal)

	// Convert []Task → []*Task for DiffTasks
	tasksA := make([]*internal.Task, len(tasksAVal))
	for i := range tasksAVal {
		tasksA[i] = &tasksAVal[i]
	}

	tasksB := make([]*internal.Task, len(tasksBVal))
	for i := range tasksBVal {
		tasksB[i] = &tasksBVal[i]
	}

	// Compute diff
	diffTasks, err := internal.DiffTasks(tasksA, tasksB, *psTreeCmd, *psTreeEnv, *files, *sockets)
	if err != nil {
		return fmt.Errorf("failed to compute diff: %w", err)
	}

	// Render output
	switch *format {
	case "tree":
		return internal.RenderDiffTreeView(diffTasks)
	case "json":
		return internal.RenderDiffJSONView(diffTasks)
	default:
		return fmt.Errorf("invalid output format: %s", *format)
	}
}
