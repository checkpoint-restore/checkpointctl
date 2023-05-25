// SPDX-License-Identifier: Apache-2.0

// This file is used to handle container checkpoint archives

package main

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
	"github.com/checkpoint-restore/go-criu/v6/crit"
	"github.com/containers/storage/pkg/archive"
	"github.com/olekukonko/tablewriter"
	spec "github.com/opencontainers/runtime-spec/specs-go"
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

func getPodmanInfo(containerConfig *metadata.ContainerConfig, _ *spec.Spec) *containerInfo {
	return &containerInfo{
		Name:    containerConfig.Name,
		Created: containerConfig.CreatedTime.Format(time.RFC3339),
		Engine:  "Podman",
	}
}

func getContainerdInfo(containerdStatus *metadata.ContainerdStatus, specDump *spec.Spec) *containerInfo {
	return &containerInfo{
		Name:    specDump.Annotations["io.kubernetes.cri.container-name"],
		Created: time.Unix(0, containerdStatus.CreatedAt).Format(time.RFC3339),
		Engine:  "containerd",
	}
}

func getCRIOInfo(_ *metadata.ContainerConfig, specDump *spec.Spec) (*containerInfo, error) {
	cm := containerMetadata{}
	if err := json.Unmarshal([]byte(specDump.Annotations["io.kubernetes.cri-o.Metadata"]), &cm); err != nil {
		return nil, fmt.Errorf("failed to read io.kubernetes.cri-o.Metadata: %w", err)
	}

	return &containerInfo{
		IP:      specDump.Annotations["io.kubernetes.cri-o.IP.0"],
		Name:    cm.Name,
		Created: specDump.Annotations["io.kubernetes.cri-o.Created"],
		Engine:  "CRI-O",
	}, nil
}

func showContainerCheckpoint(checkpointDirectory, input string) error {
	var (
		row []string
		ci  *containerInfo
	)
	containerConfig, _, err := metadata.ReadContainerCheckpointConfigDump(checkpointDirectory)
	if err != nil {
		return err
	}
	specDump, _, err := metadata.ReadContainerCheckpointSpecDump(checkpointDirectory)
	if err != nil {
		return err
	}

	switch m := specDump.Annotations["io.container.manager"]; m {
	case "libpod":
		ci = getPodmanInfo(containerConfig, specDump)
	case "cri-o":
		ci, err = getCRIOInfo(containerConfig, specDump)
	default:
		containerdStatus, _, _ := metadata.ReadContainerCheckpointStatusFile(checkpointDirectory)
		if containerdStatus == nil {
			return fmt.Errorf("unknown container manager found: %s", m)
		}
		ci = getContainerdInfo(containerdStatus, specDump)
	}

	if err != nil {
		return fmt.Errorf("getting container checkpoint information failed: %w", err)
	}

	fmt.Printf("\nDisplaying container checkpoint data from %s\n\n", input)

	table := tablewriter.NewWriter(os.Stdout)
	header := []string{
		"Container",
		"Image",
		"ID",
		"Runtime",
		"Created",
		"Engine",
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
	if ci.IP != "" {
		header = append(header, "IP")
		row = append(row, ci.IP)
	}
	if ci.MAC != "" {
		header = append(header, "MAC")
		row = append(row, ci.MAC)
	}

	archiveSizes, err := getArchiveSizes(input)
	if err != nil {
		return err
	}

	header = append(header, "CHKPT Size")
	row = append(row, metadata.ByteToString(archiveSizes.checkpointSize))

	// Display root fs diff size if available
	if archiveSizes.rootFsDiffTarSize != 0 {
		header = append(header, "Root Fs Diff Size")
		row = append(row, metadata.ByteToString(archiveSizes.rootFsDiffTarSize))
	}

	table.SetAutoMergeCells(true)
	table.SetRowLine(true)
	table.SetHeader(header)
	table.Append(row)
	table.Render()

	if showMounts {
		table = tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{
			"Destination",
			"Type",
			"Source",
		})
		// Get overview of mounts from spec.dump
		for _, data := range specDump.Mounts {
			table.Append([]string{
				data.Destination,
				data.Type,
				func() string {
					if fullPaths {
						return data.Source
					}
					return shortenPath(data.Source)
				}(),
			})
		}
		fmt.Println("\nOverview of Mounts")
		table.Render()
	}

	if printStats {
		cpDir, err := os.Open(checkpointDirectory)
		if err != nil {
			return err
		}
		defer cpDir.Close()

		// Get dump statistics with crit
		dumpStatistics, err := crit.GetDumpStats(cpDir.Name())
		if err != nil {
			return fmt.Errorf("unable to display checkpointing statistics: %w", err)
		}

		table = tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{
			"Freezing Time",
			"Frozen Time",
			"Memdump Time",
			"Memwrite Time",
			"Pages Scanned",
			"Pages Written",
		})
		table.Append([]string{
			fmt.Sprintf("%d us", dumpStatistics.GetFreezingTime()),
			fmt.Sprintf("%d us", dumpStatistics.GetFrozenTime()),
			fmt.Sprintf("%d us", dumpStatistics.GetMemdumpTime()),
			fmt.Sprintf("%d us", dumpStatistics.GetMemwriteTime()),
			fmt.Sprintf("%d", dumpStatistics.GetPagesScanned()),
			fmt.Sprintf("%d", dumpStatistics.GetPagesWritten()),
		})
		fmt.Println("\nCRIU dump statistics")
		table.Render()
	}

	return nil
}

func hasPrefix(path, prefix string) bool {
	return strings.HasPrefix(strings.TrimPrefix(path, "./"), prefix)
}

type archiveSizes struct {
	checkpointSize    int64
	rootFsDiffTarSize int64
}

// getArchiveSizes calculates the sizes of different components within a container checkpoint.
func getArchiveSizes(archiveInput string) (*archiveSizes, error) {
	result := &archiveSizes{}

	err := iterateTarArchive(archiveInput, func(hdr *tar.Header) {
		if hdr.FileInfo().Mode().IsRegular() {
			if hasPrefix(hdr.Name, metadata.CheckpointDirectory) {
				// Add the file size to the total checkpoint size
				result.checkpointSize += hdr.Size
			} else if hasPrefix(hdr.Name, metadata.RootFsDiffTar) {
				// Read the size of rootfs diff
				result.rootFsDiffTarSize = hdr.Size
			}
		}
	})
	return result, err
}

func shortenPath(path string) string {
	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) <= 2 {
		return path
	}
	return filepath.Join("..", filepath.Join(parts[len(parts)-2:]...))
}

// untarFiles unpack only specified files from an archive to the destination directory.
func untarFiles(src, dest string, files ...string) error {
	archiveFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	options := archive.TarOptions{
		ExcludePatterns: []string{
			"artifacts",
			"ctr.log",
			metadata.RootFsDiffTar,
			metadata.NetworkStatusFile,
			metadata.DeletedFilesFile,
			metadata.CheckpointDirectory,
			metadata.CheckpointVolumesDirectory,
			metadata.ConfigDumpFile,
			metadata.SpecDumpFile,
			metadata.DevShmCheckpointTar,
			metadata.DumpLogFile,
			metadata.RestoreLogFile,
		},
	}

	// Remove files from ExcludePatterns if they are present
	for _, file := range files {
		for i, pattern := range options.ExcludePatterns {
			if pattern == file {
				options.ExcludePatterns = append(options.ExcludePatterns[:i], options.ExcludePatterns[i+1:]...)
				break
			}
		}
	}

	if err := archive.Untar(archiveFile, dest, &options); err != nil {
		return fmt.Errorf("unpacking of checkpoint archive failed: %w", err)
	}

	return nil
}

// isFileInArchive checks if a file or directory with the specified pattern exists in the archive.
// It returns true if the file or directory is found, and false otherwise.
func isFileInArchive(archiveInput, pattern string, isDir bool) (bool, error) {
	found := false

	err := iterateTarArchive(archiveInput, func(hdr *tar.Header) {
		// Check if the current file or directory matches the pattern and type
		if hasPrefix(hdr.Name, pattern) && hdr.FileInfo().Mode().IsDir() == isDir {
			found = true
		}
	})
	return found, err
}

// iterateTarArchive reads a tar archive from the specified input file,
// decompresses it, and iterates through each entry, invoking the provided callback function.
func iterateTarArchive(archiveInput string, callback func(hdr *tar.Header)) error {
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
		hdr, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		callback(hdr)
	}

	return nil
}
