// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const (
	pluginPrefix              = "checkpointctl-"
	pluginDescriptionFlag     = "--plugin-description"
	pluginDescriptionTimeout  = 500 * time.Millisecond
	pluginDescriptionExitCode = 42
)

// Plugin represents a discovered external plugin.
type Plugin struct {
	Name        string // subcommand name (e.g., "build")
	Path        string // full path to executable
	Description string // description provided by the plugin
}

// getPluginDescription queries the plugin for its description by running
// it with --plugin-description flag. Returns empty string if the plugin
// doesn't support this flag (must exit with code 42) or fails to respond in time.
func getPluginDescription(pluginPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), pluginDescriptionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, pluginPath, pluginDescriptionFlag)
	output, err := cmd.Output()
	// Plugin must exit with code 42 to indicate it supports --plugin-description
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == pluginDescriptionExitCode {
				// Exit code 42 with output means description is supported
				desc := strings.TrimSpace(string(output))
				if idx := strings.Index(desc, "\n"); idx != -1 {
					desc = desc[:idx]
				}
				return desc
			}
		}
		return ""
	}

	// Exit code 0 does not indicate description support
	return ""
}

// DiscoverPlugins searches PATH for checkpointctl-* executables.
func DiscoverPlugins() []Plugin {
	var plugins []Plugin
	seen := make(map[string]bool)

	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()
			if !strings.HasPrefix(name, pluginPrefix) {
				continue
			}

			// Extract subcommand name from checkpointctl-<name>
			subcommand := strings.TrimPrefix(name, pluginPrefix)
			if subcommand == "" {
				continue
			}

			// Skip if already found (first in PATH wins)
			if seen[subcommand] {
				continue
			}

			fullPath := filepath.Join(dir, name)
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}

			// Check if executable
			if info.Mode()&0o111 == 0 {
				continue
			}

			// Convert to absolute path to ensure exec.Command works correctly
			// (relative paths without "/" are looked up in PATH)
			absPath, err := filepath.Abs(fullPath)
			if err != nil {
				absPath = fullPath
			}

			seen[subcommand] = true
			plugins = append(plugins, Plugin{
				Name:        subcommand,
				Path:        absPath,
				Description: getPluginDescription(absPath),
			})
		}
	}

	return plugins
}

// CreatePluginCommand creates a cobra.Command that executes the plugin.
func CreatePluginCommand(plugin Plugin) *cobra.Command {
	short := plugin.Description
	if short == "" {
		short = fmt.Sprintf("Plugin provided by %s", plugin.Path)
	}

	return &cobra.Command{
		Use:                plugin.Name,
		Short:              short,
		Long:               fmt.Sprintf("External plugin command provided by %s", plugin.Path),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ExecutePlugin(plugin.Path, args)
		},
	}
}

// ExecutePlugin runs the plugin binary with the given arguments.
// It uses syscall.Exec to replace the current process with the plugin,
// which provides proper signal handling and exit code propagation.
func ExecutePlugin(pluginPath string, args []string) error {
	return syscall.Exec(pluginPath, append([]string{pluginPath}, args...), os.Environ())
}

// PluginList returns a command that lists all available plugins.
func PluginList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			plugins := DiscoverPlugins()
			if len(plugins) == 0 {
				fmt.Println("No plugins found in PATH")
				return nil
			}
			fmt.Println("Available plugins:")
			for _, p := range plugins {
				desc := p.Description
				if desc == "" {
					desc = "(no description)"
				}
				fmt.Printf("  %-20s %s\n", p.Name, desc)
				fmt.Printf("  %-20s %s\n", "", p.Path)
			}
			return nil
		},
	}
}

// PluginCmd returns the parent command for plugin management.
func PluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage checkpointctl plugins",
	}
	cmd.AddCommand(PluginList())
	return cmd
}
