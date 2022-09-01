// SPDX-License-Identifier: Apache-2.0

// This file is used to handle pod checkpoint archives

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
)

func showPodCheckpoint(checkpointDirectory string) error {
	podSandboxConfig, podSandboxConfigFile, err := metadata.ReadPodCheckpointDumpFile(checkpointDirectory)
	if err != nil {
		return errors.Wrapf(err, "reading %q failed", podSandboxConfigFile)
	}

	checkpointedPodOptions, checkpointedPodOptionsFile, err := metadata.ReadPodCheckpointOptionsFile(checkpointDirectory)
	if err != nil {
		return errors.Wrapf(err, "reading %q failed", checkpointedPodOptionsFile)
	}

	fmt.Printf("\nDisplaying pod checkpoint data from %s\n\n", kubeletCheckpointsDirectory)

	table := tablewriter.NewWriter(os.Stdout)

	header := []string{
		"Pod",
		"Namespace",
		"Hostname",
		"Container",
	}

	table.SetAutoMergeCells(true)
	table.SetRowLine(true)

	if showPodUID {
		header = append(header, "Pod UID")
	}

	sizeHeader := false

	for _, p := range checkpointedPodOptions.Containers {
		var row []string
		var name string

		row = append(row, podSandboxConfig.Metadata.Name)
		row = append(row, podSandboxConfig.Metadata.Namespace)
		row = append(row, podSandboxConfig.Hostname)
		names := strings.Split(p, "_")
		if len(names) > 5 {
			name = names[1]
		} else {
			name = p
		}

		row = append(row, name)

		if showPodUID {
			row = append(row, podSandboxConfig.Metadata.UID)
		}

		fi, err := os.Lstat(filepath.Join(checkpointDirectory, p+".tar"))
		if err == nil {
			if fi.Size() != 0 {
				if !sizeHeader {
					header = append(header, "CHKPT Size")
					sizeHeader = true
				}
				row = append(row, metadata.ByteToString(fi.Size()))
			}
		}
		table.Append(row)
	}

	table.SetHeader(header)
	table.Render()

	return nil
}
