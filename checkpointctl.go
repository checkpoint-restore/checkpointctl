// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	cmd "github.com/checkpoint-restore/checkpointctl/cmd"
	"github.com/spf13/cobra"
)

var (
	name    string
	version string
)

func main() {
	rootCommand := &cobra.Command{
		Use:   name,
		Short: name + " is a tool to read and manipulate checkpoint archives",
		Long: name + " is a tool to read and manipulate checkpoint archives as " +
			"created by Podman, CRI-O and containerd",
		SilenceUsage: true,
	}

	rootCommand.AddCommand(cmd.Show())

	rootCommand.AddCommand(cmd.Inspect())

	rootCommand.AddCommand(cmd.MemParse())

	rootCommand.AddCommand(cmd.List())

	rootCommand.Version = version

	if err := rootCommand.Execute(); err != nil {
		os.Exit(1)
	}
}
