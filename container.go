// SPDX-License-Identifier: Apache-2.0

// This file is used to handle container checkpoint archives

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/olekukonko/tablewriter"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

type containerMetadata struct {
	Name    string `json:"name,omitempty"`
	Attempt uint32 `json:"attempt,omitempty"`
}

type containerInfo struct {
	Name    string
	IP      string
	MAC     string
	Created string
	Engine  string
}

func getPodmanInfo(
	containerConfig *metadata.ContainerConfig,
	specDump *spec.Spec, checkpointDirectory string,
) (*containerInfo, error) {
	ci := &containerInfo{}

	ci.Name = containerConfig.Name
	ci.Created = containerConfig.CreatedTime.Format(time.RFC3339)
	ci.Engine = "Podman"

	return ci, nil
}

func getCRIOInfo(containerConfig *metadata.ContainerConfig, specDump *spec.Spec) (*containerInfo, error) {
	ci := &containerInfo{}

	ci.IP = specDump.Annotations["io.kubernetes.cri-o.IP.0"]

	cm := containerMetadata{}
	if err := json.Unmarshal([]byte(specDump.Annotations["io.kubernetes.cri-o.Metadata"]), &cm); err != nil {
		return ci, errors.Wrapf(err, "Failed to read io.kubernetes.cri-o.Metadata")
	}

	ci.Name = cm.Name
	ci.Created = specDump.Annotations["io.kubernetes.cri-o.Created"]
	ci.Engine = "CRI-O"

	return ci, nil
}

func showContainerCheckpoint(checkpointDirectory string) error {
	var (
		row []string
		ci  *containerInfo
	)
	containerConfig, configDumpFile, err := metadata.ReadContainerCheckpointConfigDump(checkpointDirectory)
	if err != nil {
		return errors.Wrapf(err, "Reading %q failed\n", configDumpFile)
	}
	specDump, specDumpFile, err := metadata.ReadContainerCheckpointSpecDump(checkpointDirectory)
	if err != nil {
		return errors.Wrapf(err, "Reading %q failed\n", specDumpFile)
	}

	switch specDump.Annotations["io.container.manager"] {
	case "libpod":
		ci, err = getPodmanInfo(containerConfig, specDump, checkpointDirectory)
	case "cri-o":
		ci, err = getCRIOInfo(containerConfig, specDump)
	default:
		return errors.Errorf("Unknown container manager found: %s", specDump.Annotations["io.container.manager"])
	}

	if err != nil {
		return errors.Wrap(err, "Getting container checkpoint information failed")
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
	}

	row = append(row, ci.Name)
	row = append(row, containerConfig.RootfsImageName)
	if len(containerConfig.ID) > 12 {
		row = append(row, containerConfig.ID[:12])
	} else {
		row = append(row, containerConfig.ID)
	}

	row = append(row, containerConfig.OCIRuntime)
	row = append(row, ci.Created)

	row = append(row, ci.Engine)
	row = append(row, ci.IP)
	if ci.MAC != "" {
		header = append(header, "MAC")
		row = append(row, ci.MAC)
	}

	size, err := getCheckpointSize(checkpointDirectory)
	if err != nil {
		return err
	}

	header = append(header, "CHKPT Size")
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

func dirSize(path string) (size int64, err error) {
	err = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}

		return err
	})

	return size, err
}

func getCheckpointSize(path string) (size int64, err error) {
	dir := filepath.Join(path, metadata.CheckpointDirectory)

	return dirSize(dir)
}
