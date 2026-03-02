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
	requiredFiles := []string{metadata.SpecDumpFile, metadata.ConfigDumpFile, metadata.NetworkStatusFile}
	tasks, err := internal.CreateTasks(args, requiredFiles)
	if err != nil {
		return err
	}
	defer internal.CleanupTasks(tasks)

	return internal.ShowContainerCheckpoints(tasks)
}
