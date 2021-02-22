// SPDX-License-Identifier: Apache-2.0

// This file is used to handle container checkpoint archives

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
)

func showContainerCheckpoint(checkpointDirectory string) error {
	err, containerConfig, configDumpFile := metadata.ReadContainerCheckpointConfigDump(checkpointDirectory)
	if err != nil {
		return errors.Wrapf(err, "Reading %q failed\n", configDumpFile)
	}
	err, specDump, specDumpFile := metadata.ReadContainerCheckpointSpecDump(checkpointDirectory)
	if err != nil {
		return errors.Wrapf(err, "Reading %q failed\n", specDumpFile)
	}
	err, networkStatus, networkStatusFile := metadata.ReadContainerCheckpointNetworkStatus(checkpointDirectory)
	if err != nil {
		return errors.Wrapf(err, "Reading %q failed\n", networkStatusFile)
	}

	fmt.Printf("\nDisplaying container checkpoint data from %s\n\n", kubeletCheckpointsDirectory)

	table := tablewriter.NewWriter(os.Stdout)
	header := []string{
		"Container",
		"Image",
		"ID",
		"Runtime",
		"Created",
		"Engine",
		"IP",
		"MAC",
		"CHKPT Size",
	}
	var row []string
	row = append(row, containerConfig.Name)
	row = append(row, containerConfig.RootfsImageName)
	if len(containerConfig.ID) > 12 {
		row = append(row, containerConfig.ID[:12])
	} else {
		row = append(row, containerConfig.ID)
	}
	row = append(row, containerConfig.OCIRuntime)
	row = append(row, containerConfig.CreatedTime.Format(time.RFC3339))
	if specDump.Annotations["io.container.manager"] == "libpod" {
		row = append(row, "Podman")
	} else {
		row = append(row, "Unknown")
	}

	if IP := metadata.GetIPFromNetworkStatus(networkStatus); IP != nil {
		row = append(row, IP.String())
	} else {
		row = append(row, "Unknown")
	}

	if MAC := metadata.GetMACFromNetworkStatus(networkStatus); MAC != nil {
		row = append(row, MAC.String())
	} else {
		row = append(row, "Unknown")
	}

	err, size := getCheckpointSize(checkpointDirectory)
	if err != nil {
		return err
	}

	row = append(row, metadata.ByteToString(size))

	// Display root fs diff size if available

	fi, err := os.Lstat(filepath.Join(checkpointDirectory, metadata.RootFsDiffTar))
	if err == nil {
		if fi.Size() != 0 {
			header = append(header, "Root Fs Diff Size")
			row = append(row, metadata.ByteToString(fi.Size()))
		}
	}

	table.SetAutoMergeCells(true)
	table.SetRowLine(true)
	table.SetHeader(header)
	table.Append(row)
	table.Render()

	return nil
}

func dirSize(path string) (err error, size int64) {
	err = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return err, size
}

func getCheckpointSize(path string) (err error, size int64) {
	dir := filepath.Join(path, metadata.CheckpointDirectory)
	return dirSize(dir)
}
