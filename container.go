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
	"github.com/checkpoint-restore/go-criu/v6/crit/images"
	"github.com/containers/storage/pkg/archive"
	"github.com/olekukonko/tablewriter"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/xlab/treeprint"
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
	containerConfig, _, err := metadata.ReadContainerCheckpointConfigDump(checkpointDirectory)
	if err != nil {
		return err
	}
	specDump, _, err := metadata.ReadContainerCheckpointSpecDump(checkpointDirectory)
	if err != nil {
		return err
	}

	var ci *containerInfo
	switch m := specDump.Annotations["io.container.manager"]; m {
	case "libpod":
		ci = getPodmanInfo(containerConfig, specDump)
	case "cri-o":
		ci, err = getCRIOInfo(containerConfig, specDump)
		if err != nil {
			return fmt.Errorf("getting container checkpoint information failed: %w", err)
		}
	default:
		containerdStatus, _, _ := metadata.ReadContainerCheckpointStatusFile(checkpointDirectory)
		if containerdStatus == nil {
			return fmt.Errorf("unknown container manager found: %s", m)
		}
		ci = getContainerdInfo(containerdStatus, specDump)
	}

	// Fetch root fs diff size if available
	archiveSizes, err := getArchiveSizes(input)
	if err != nil {
		return fmt.Errorf("failed to get archive sizes: %w", err)
	}

	fmt.Printf("\nDisplaying container checkpoint data from %s\n\n", input)

	renderCheckpoint(ci, containerConfig, archiveSizes)

	if mounts {
		renderMounts(specDump)
	}

	if stats {
		// Get dump statistics with crit
		dumpStats, err := crit.GetDumpStats(checkpointDirectory)
		if err != nil {
			return fmt.Errorf("failed to get dump statistics: %w", err)
		}

		renderDumpStats(dumpStats)
	}

	if psTree {
		// The image files reside in a subdirectory called "checkpoint"
		c := crit.New("", "", filepath.Join(checkpointDirectory, "checkpoint"), false, false)
		// Get process tree with CRIT
		psTree, err := c.ExplorePs()
		if err != nil {
			return fmt.Errorf("failed to get process tree: %w", err)
		}

		renderPsTree(psTree, ci.Name)
	}

	return nil
}

func renderCheckpoint(ci *containerInfo, containerConfig *metadata.ContainerConfig, archiveSizes *archiveSizes) {
	table := tablewriter.NewWriter(os.Stdout)
	header := []string{
		"Container",
		"Image",
		"ID",
		"Runtime",
		"Created",
		"Engine",
	}

	var row []string
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
}

func renderMounts(specDump *spec.Spec) {
	table := tablewriter.NewWriter(os.Stdout)
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

func renderDumpStats(dumpStats *images.DumpStatsEntry) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{
		"Freezing Time",
		"Frozen Time",
		"Memdump Time",
		"Memwrite Time",
		"Pages Scanned",
		"Pages Written",
	})
	table.Append([]string{
		fmt.Sprintf("%d us", dumpStats.GetFreezingTime()),
		fmt.Sprintf("%d us", dumpStats.GetFrozenTime()),
		fmt.Sprintf("%d us", dumpStats.GetMemdumpTime()),
		fmt.Sprintf("%d us", dumpStats.GetMemwriteTime()),
		fmt.Sprintf("%d", dumpStats.GetPagesScanned()),
		fmt.Sprintf("%d", dumpStats.GetPagesWritten()),
	})
	fmt.Println("\nCRIU dump statistics")
	table.Render()
}

func renderPsTree(psTree *crit.PsTree, containerName string) {
	var tree treeprint.Tree
	if containerName == "" {
		containerName = "Container"
	}
	tree = treeprint.NewWithRoot(containerName)
	// processNodes is a recursive function to create
	// a new branch for each process and add its child
	// processes as child nodes of the branch.
	var processNodes func(treeprint.Tree, *crit.PsTree)
	processNodes = func(tree treeprint.Tree, root *crit.PsTree) {
		node := tree.AddMetaBranch(root.PId, root.Comm)
		for _, child := range root.Children {
			processNodes(node, child)
		}
	}

	processNodes(tree, psTree)

	fmt.Print("\nProcess tree\n\n")
	fmt.Println(tree.String())
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

	err := iterateTarArchive(archiveInput, func(r *tar.Reader, header *tar.Header) error {
		if header.FileInfo().Mode().IsRegular() {
			if hasPrefix(header.Name, metadata.CheckpointDirectory) {
				// Add the file size to the total checkpoint size
				result.checkpointSize += header.Size
			} else if hasPrefix(header.Name, metadata.RootFsDiffTar) {
				// Read the size of rootfs diff
				result.rootFsDiffTarSize = header.Size
			}
		}
		return nil
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
func untarFiles(src, dest string, files []string) error {
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
				if err := os.MkdirAll(filepath.Join(dest, filepath.Dir(header.Name)), 0o644); err != nil {
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
		return fmt.Errorf("unpacking of checkpoint archive failed: %w", err)
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
