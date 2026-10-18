// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/checkpoint-restore/checkpointctl/internal"
	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/spf13/cobra"
)

var tcpListenRemap *string

func Remap() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remap <checkpoint-path>",
		Short: "Remap TCP listen ports in a container checkpoint archive",
		Long: `The 'remap' command allows remapping TCP listen ports in a checkpoint archive.
This is useful when the port originally bound to a socket needs to be changed before restoring.

Port mappings are specified using the --tcp-listen-remap flag in the format:
  old_port:new_port

Multiple port mappings can be specified by separating them with commas:
  --tcp-listen-remap 8080:80,8443:443

Example:
  checkpointctl remap checkpoint.tar --tcp-listen-remap 8080:80`,
		Args: cobra.ExactArgs(1),
		RunE: remapTCPPorts,
	}

	tcpListenRemap = cmd.Flags().String(
		"tcp-listen-remap",
		"",
		"TCP listen port remapping in format old_port:new_port (e.g., 8080:80). Multiple mappings can be separated by commas.",
	)

	cmd.MarkFlagRequired("tcp-listen-remap")

	return cmd
}

func remapTCPPorts(cmd *cobra.Command, args []string) error {
	checkpointPath := args[0]

	// Parse port mappings
	portMappings, err := parsePortMappings(*tcpListenRemap)
	if err != nil {
		return fmt.Errorf("failed to parse port mappings: %w", err)
	}

	if len(portMappings) == 0 {
		return fmt.Errorf("no valid port mappings specified")
	}

	// Create temporary directory for extraction
	tempDir, err := os.MkdirTemp("", "checkpointctl-remap-")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract files.img from checkpoint
	filesImgPath := filepath.Join(metadata.CheckpointDirectory, "files.img")
	if err := internal.UntarFiles(checkpointPath, tempDir, []string{filesImgPath}); err != nil {
		return fmt.Errorf("failed to extract files.img from checkpoint: %w", err)
	}

	extractedFilesImgPath := filepath.Join(tempDir, filesImgPath)

	// Perform port remapping
	modified, err := internal.RemapTCPListenPorts(extractedFilesImgPath, portMappings)
	if err != nil {
		return fmt.Errorf("failed to remap TCP ports: %w", err)
	}

	if !modified {
		fmt.Println("No TCP listen sockets were found matching the specified port mappings")
		return nil
	}

	// Repack the modified files.img back into the checkpoint archive
	if err := internal.RepackFileToArchive(checkpointPath, filesImgPath, extractedFilesImgPath); err != nil {
		return fmt.Errorf("failed to repack checkpoint archive: %w", err)
	}

	fmt.Printf("Successfully remapped TCP listen ports in %s\n", checkpointPath)
	return nil
}

func parsePortMappings(mappingStr string) (map[uint32]uint32, error) {
	mappings := make(map[uint32]uint32)

	if mappingStr == "" {
		return mappings, nil
	}

	parts := strings.Split(mappingStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		ports := strings.Split(part, ":")
		if len(ports) != 2 {
			return nil, fmt.Errorf("invalid port mapping format: %s (expected old_port:new_port)", part)
		}

		oldPort, err := strconv.ParseUint(strings.TrimSpace(ports[0]), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid old port '%s': %w", ports[0], err)
		}

		newPort, err := strconv.ParseUint(strings.TrimSpace(ports[1]), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid new port '%s': %w", ports[1], err)
		}

		if oldPort == 0 || oldPort > 65535 {
			return nil, fmt.Errorf("old port %d is out of valid range (1-65535)", oldPort)
		}
		if newPort == 0 || newPort > 65535 {
			return nil, fmt.Errorf("new port %d is out of valid range (1-65535)", newPort)
		}

		mappings[uint32(oldPort)] = uint32(newPort)
	}

	return mappings, nil
}