package cmd

import (
	"fmt"

	"github.com/checkpoint-restore/checkpointctl/internal"
	"github.com/spf13/cobra"
)

var tcpListenRemapFlag string

func EditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <archive-path>",
		Short: "Edit a checkpoint archive",
		Long: `The 'edit' command can help you change the properties of a container inside a checkpoint archive.
Currently only supports remapping the TCP listen ports.
Example:
	checkpointctl edit --tcp-listen-remap 8080:80 checkpoint.tar`,
		Args: cobra.ExactArgs(1),
		RunE: editArchive,
	}

	cmd.Flags().StringVar(
		&tcpListenRemapFlag,
		"tcp-listen-remap",
		"",
		"Remap TCP listen port (format: oldport:newport)",
	)

	return cmd
}

func editArchive(cmd *cobra.Command, args []string) error {
	archivePath := args[0]

	if tcpListenRemapFlag != "" {
		return internal.TcpListenRemap(tcpListenRemapFlag, archivePath)
	}

	return fmt.Errorf("no edit operation specified; use --tcp-listen-remap")
}
