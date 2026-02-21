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
	rootCommand.AddCommand(cmd.BuildCmd())
	rootCommand.AddCommand(cmd.PluginCmd())

	// Discover and register external plugins from PATH.
	// Plugins are executables named checkpointctl-<name> where <name>
	// becomes a subcommand. Built-in commands take precedence.
	builtinCommands := getBuiltinCommandNames(rootCommand)
	for _, plugin := range cmd.DiscoverPlugins() {
		// Skip plugins that would shadow built-in commands
		if builtinCommands[plugin.Name] {
			continue
		}
		rootCommand.AddCommand(cmd.CreatePluginCommand(plugin))
	}

	rootCommand.AddCommand(cmd.Diff())

	rootCommand.Version = version

	if err := rootCommand.Execute(); err != nil {
		os.Exit(1)
	}
}

// getBuiltinCommandNames returns a set of command names already registered.
func getBuiltinCommandNames(root *cobra.Command) map[string]bool {
	names := make(map[string]bool)
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	return names
}
