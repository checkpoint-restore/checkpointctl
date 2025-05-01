// SPDX-License-Identifier: Apache-2.0

// This file is used to handle container checkpoint archives

package internal

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v7/crit"
	"github.com/containers/storage/pkg/archive"
	"github.com/olekukonko/tablewriter"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

var pageSize = os.Getpagesize()

type containerMetadata struct {
	Name    string `json:"name,omitempty"`
	Attempt uint32 `json:"attempt,omitempty"`
}

type containerInfo struct {
	Name      string
	IP        string
	MAC       string
	Created   string
	Engine    string
	Namespace string
	Pod       string
}

type checkpointInfo struct {
	containerInfo *containerInfo
	specDump      *specs.Spec
	configDump    *metadata.ContainerConfig
	archiveSizes  *archiveSizes
}

func getPodmanInfo(containerConfig *metadata.ContainerConfig, specDump *specs.Spec, task Task) *containerInfo {
	info := &containerInfo{
		Name:    containerConfig.Name,
		Created: containerConfig.CreatedTime.Format(time.RFC3339),
		Engine:  "Podman",
	}

	// Try to get network information from network.status file
	if specDump.Annotations["io.container.manager"] == "libpod" {
		// Create temp dir for network status file
		tmpDir, err := os.MkdirTemp("", "network-status")
		if err == nil {
			defer os.RemoveAll(tmpDir)
			
			// Extract network.status file
			err = UntarFiles(task.CheckpointFilePath, tmpDir, []string{metadata.NetworkStatusFile})
			if err == nil {
				networkStatusFile := filepath.Join(tmpDir, metadata.NetworkStatusFile)
				ip, mac, err := getPodmanNetworkInfo(networkStatusFile)
				if err == nil {
					info.IP = ip
					info.MAC = mac
				}
			}
		}
	}

	return info
}

func getContainerdInfo(containerConfig *metadata.ContainerConfig, specDump *specs.Spec) *containerInfo {
	return &containerInfo{
		Name:      specDump.Annotations["io.kubernetes.cri.container-name"],
		Created:   containerConfig.CreatedTime.Format(time.RFC3339),
		Engine:    "containerd",
		Namespace: specDump.Annotations["io.kubernetes.cri.sandbox-namespace"],
		Pod:       specDump.Annotations["io.kubernetes.cri.sandbox-name"],
	}
}

func getCRIOInfo(_ *metadata.ContainerConfig, specDump *specs.Spec) (*containerInfo, error) {
	cm := containerMetadata{}
	if err := json.Unmarshal([]byte(specDump.Annotations["io.kubernetes.cri-o.Metadata"]), &cm); err != nil {
		return nil, fmt.Errorf("failed to read io.kubernetes.cri-o.Metadata: %w", err)
	}

	return &containerInfo{
		IP:        specDump.Annotations["io.kubernetes.cri-o.IP.0"],
		Name:      cm.Name,
		Created:   specDump.Annotations["io.kubernetes.cri-o.Created"],
		Engine:    "CRI-O",
		Namespace: specDump.Annotations["io.kubernetes.pod.namespace"],
		Pod:       specDump.Annotations["io.kubernetes.pod.name"],
	}, nil
}

func getCheckpointInfo(task Task) (*checkpointInfo, error) {
	info := &checkpointInfo{}
	var err error

	info.configDump, _, err = metadata.ReadContainerCheckpointConfigDump(task.OutputDir)
	if err != nil {
		if strings.Contains(err.Error(), "unexpected end of JSON input") {
			return nil, fmt.Errorf("config.dump: unexpected end of JSON input")
		}
		return nil, fmt.Errorf("config.dump: %w", err)
	}
	info.specDump, _, err = metadata.ReadContainerCheckpointSpecDump(task.OutputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("spec.dump: no such file or directory")
		}
		if strings.Contains(err.Error(), "unexpected end of JSON input") {
			return nil, fmt.Errorf("spec.dump: unexpected end of JSON input")
		}
		return nil, fmt.Errorf("spec.dump: %w", err)
	}

	info.containerInfo, err = getContainerInfo(info.specDump, info.configDump, task)
	if err != nil {
		return nil, err
	}

	info.archiveSizes, err = getArchiveSizes(task.CheckpointFilePath)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func ShowContainerCheckpoints(tasks []Task) error {
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
		"Root FS Diff Size",
	}

	for _, task := range tasks {
		info, err := getCheckpointInfo(task)
		if err != nil {
			if strings.Contains(err.Error(), "Error: ") {
				return fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "Error: "))
			}
			return err
		}

		if len(tasks) == 1 {
			fmt.Printf("Displaying container checkpoint data from %s\n", task.CheckpointFilePath)
		}

		var row []string
		row = append(row, info.containerInfo.Name)
		row = append(row, info.configDump.RootfsImageName)
		if len(info.configDump.ID) > 12 {
			row = append(row, info.configDump.ID[:12])
		} else {
			row = append(row, info.configDump.ID)
		}
		row = append(row, info.configDump.OCIRuntime)
		row = append(row, info.containerInfo.Created)
		row = append(row, info.containerInfo.Engine)
		row = append(row, info.containerInfo.IP)
		row = append(row, info.containerInfo.MAC)
		row = append(row, metadata.ByteToString(info.archiveSizes.checkpointSize))
		row = append(row, metadata.ByteToString(info.archiveSizes.rootFsDiffTarSize))

		table.Append(row)
	}

	table.SetHeader(header)
	table.SetAutoMergeCells(false)
	table.SetRowLine(true)
	table.Render()

	return nil
}

func getContainerInfo(specDump *specs.Spec, containerConfig *metadata.ContainerConfig, task Task) (*containerInfo, error) {
	var ci *containerInfo
	switch m := specDump.Annotations["io.container.manager"]; m {
	case "libpod":
		ci = getPodmanInfo(containerConfig, specDump, task)
	case "cri-o":
		var err error
		ci, err = getCRIOInfo(containerConfig, specDump)
		if err != nil {
			return nil, fmt.Errorf("getting container checkpoint information failed: %w", err)
		}
	default:
		ci = getContainerdInfo(containerConfig, specDump)
	}

	return ci, nil
}

func hasPrefix(path, prefix string) bool {
	return strings.HasPrefix(strings.TrimPrefix(path, "./"), prefix)
}

type archiveSizes struct {
	checkpointSize    int64
	rootFsDiffTarSize int64
	pagesSize         int64
	amdgpuPagesSize   int64
}

// getArchiveSizes calculates the sizes of different components within a container checkpoint.
func getArchiveSizes(archiveInput string) (*archiveSizes, error) {
	result := &archiveSizes{}

	err := iterateTarArchive(archiveInput, func(r *tar.Reader, header *tar.Header) error {
		if header.FileInfo().Mode().IsRegular() {
			if hasPrefix(header.Name, metadata.CheckpointDirectory) {
				// Add the file size to the total checkpoint size
				result.checkpointSize += header.Size
				if hasPrefix(header.Name, filepath.Join(metadata.CheckpointDirectory, metadata.PagesPrefix)) {
					result.pagesSize += header.Size
				} else if hasPrefix(header.Name, filepath.Join(metadata.CheckpointDirectory, metadata.AmdgpuPagesPrefix)) {
					result.amdgpuPagesSize += header.Size
				}
			} else if hasPrefix(header.Name, metadata.RootFsDiffTar) {
				// Read the size of rootfs diff
				result.rootFsDiffTarSize = header.Size
			}
		}
		return nil
	})
	return result, err
}

// UntarFiles unpack only specified files from an archive to the destination directory.
func UntarFiles(src, dest string, files []string) error {
	archiveFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	if err := iterateTarArchive(src, func(r *tar.Reader, header *tar.Header) error {
		// Check if the current entry is one of the target files
		for _, file := range files {
			if strings.Contains(header.Name, file) {
				// Create the destination folder
				if err := os.MkdirAll(filepath.Join(dest, filepath.Dir(header.Name)), 0o700); err != nil {
					return err
				}
				// Create the destination file
				destFile, err := os.Create(filepath.Join(dest, header.Name))
				if err != nil {
					return err
				}
				defer destFile.Close()

				// Copy the contents of the entry to the destination file
				_, err = io.Copy(destFile, r)
				if err != nil {
					return err
				}

				// File successfully extracted, move to the next file
				break
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// isFileInArchive checks if a file or directory with the specified pattern exists in the archive.
// It returns true if the file or directory is found, and false otherwise.
func isFileInArchive(archiveInput, pattern string, isDir bool) (bool, error) {
	found := false

	err := iterateTarArchive(archiveInput, func(_ *tar.Reader, header *tar.Header) error {
		// Check if the current file or directory matches the pattern and type
		if hasPrefix(header.Name, pattern) && header.FileInfo().Mode().IsDir() == isDir {
			found = true
		}
		return nil
	})
	return found, err
}

// iterateTarArchive reads a tar archive from the specified input file,
// decompresses it, and iterates through each entry, invoking the provided callback function.
func iterateTarArchive(archiveInput string, callback func(r *tar.Reader, header *tar.Header) error) error {
	archiveFile, err := os.Open(archiveInput)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	// Decompress the archive
	stream, err := archive.DecompressStream(archiveFile)
	if err != nil {
		return err
	}
	defer stream.Close()

	// Create a tar reader to read the files from the decompressed archive
	tarReader := tar.NewReader(stream)

	for {
		header, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		if err = callback(tarReader, header); err != nil {
			return err
		}
	}

	return nil
}

func getCmdline(checkpointOutputDir string, pid uint32) (cmdline string, err error) {
	mr, err := crit.NewMemoryReader(filepath.Join(checkpointOutputDir, metadata.CheckpointDirectory), pid, pageSize)
	if err != nil {
		return
	}

	buffer, err := mr.GetPsArgs()
	if err != nil {
		return
	}

	cmdline = strings.Join(strings.Split(buffer.String(), "\x00"), " ")
	return
}

func getPsEnvVars(checkpointOutputDir string, pid uint32) (envVars []string, err error) {
	mr, err := crit.NewMemoryReader(filepath.Join(checkpointOutputDir, metadata.CheckpointDirectory), pid, pageSize)
	if err != nil {
		return
	}

	buffer, err := mr.GetPsEnvVars()
	if err != nil {
		return
	}

	for _, envVar := range strings.Split(buffer.String(), "\x00") {
		if envVar != "" {
			envVars = append(envVars, envVar)
		}
	}

	return
}
