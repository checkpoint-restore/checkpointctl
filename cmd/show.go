package cmd

import (
	"github.com/checkpoint-restore/checkpointctl/internal"
	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/spf13/cobra"
)

func Show() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "show",
		Short:                 "Show an overview of container checkpoints",
		RunE:                  show,
		Args:                  cobra.MinimumNArgs(1),
		DisableFlagsInUseLine: true,
	}

	return cmd
}

func show(cmd *cobra.Command, args []string) error {
	// Only "spec.dump" and "config.dump" are need when for the show sub-command
	requiredFiles := []string{metadata.SpecDumpFile, metadata.ConfigDumpFile}
	tasks, err := internal.CreateTasks(args, requiredFiles)
	if err != nil {
		return err
	}
	defer internal.CleanupTasks(tasks)

	return internal.ShowContainerCheckpoints(tasks)
}
