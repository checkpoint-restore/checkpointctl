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
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v8/crit"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/storage/pkg/archive"
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
	Networks  []NetworkNode
	Created   string
	Engine    string
	Namespace string
	Pod       string
}

type checkpointInfo struct {
	containerInfo *containerInfo
	specDump      *spec.Spec
	configDump    *metadata.ContainerConfig
	archiveSizes  *archiveSizes
}

func getPodmanInfo(containerConfig *metadata.ContainerConfig, _ *spec.Spec, checkpointDirectory string) *containerInfo {
	ci := &containerInfo{
		Name:    containerConfig.Name,
		Created: containerConfig.CreatedTime.Format(time.RFC3339),
		Engine:  "Podman",
	}

	networkStatus, _, err := metadata.ReadContainerCheckpointNetworkStatus(checkpointDirectory)
	if err != nil {
		return ci
	}

	var ips []string
	var macs []string
	var networks []NetworkNode

	// Sort network names for deterministic output
	networkNames := make([]string, 0, len(*networkStatus))
	for name := range *networkStatus {
		networkNames = append(networkNames, name)
	}
	sort.Strings(networkNames)

	for _, name := range networkNames {
		network := (*networkStatus)[name]
		networkNode := NetworkNode{
			Name:       name,
			Interfaces: make(map[string]NetworkInterfaceNode),
		}

		// Sort interface names for deterministic output
		ifaceNames := make([]string, 0, len(network.Interfaces))
		for ifName := range network.Interfaces {
			ifaceNames = append(ifaceNames, ifName)
		}
		sort.Strings(ifaceNames)

		for _, ifName := range ifaceNames {
			iface := network.Interfaces[ifName]
			ifaceNode := NetworkInterfaceNode{}

			if iface.MacAddress != "" {
				macs = append(macs, iface.MacAddress)
				ifaceNode.MAC = iface.MacAddress
			}
			for _, subnet := range iface.Subnets {
				if subnet.IPNet != "" {
					ifaceNode.IP = subnet.IPNet
					ip := subnet.IPNet
					if idx := strings.Index(ip, "/"); idx != -1 {
						ip = ip[:idx]
					}
					if ip != "" {
						ips = append(ips, ip)
					}
				}
				if subnet.Gateway != "" {
					ifaceNode.Gateway = subnet.Gateway
				}
			}

			networkNode.Interfaces[ifName] = ifaceNode
		}

		networks = append(networks, networkNode)
	}

	ci.IP = strings.Join(ips, ", ")
	ci.MAC = strings.Join(macs, ", ")
	ci.Networks = networks

	return ci
}

func getContainerdInfo(containerConfig *metadata.ContainerConfig, specDump *spec.Spec) *containerInfo {
	return &containerInfo{
		Name:      specDump.Annotations["io.kubernetes.cri.container-name"],
		Created:   containerConfig.CreatedTime.Format(time.RFC3339),
		Engine:    "containerd",
		Namespace: specDump.Annotations["io.kubernetes.cri.sandbox-namespace"],
		Pod:       specDump.Annotations["io.kubernetes.cri.sandbox-name"],
	}
}

func getCRIOInfo(_ *metadata.ContainerConfig, specDump *spec.Spec) (*containerInfo, error) {
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
		return nil, err
	}
	info.specDump, _, err = metadata.ReadContainerCheckpointSpecDump(task.OutputDir)
	if err != nil {
		return nil, err
	}

	info.containerInfo, err = getContainerInfo(info.specDump, info.configDump, task.OutputDir)
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
	w := GetNewTabWriter(os.Stdout)

	header := []string{
		"Container",
		"Image",
		"ID",
		"Runtime",
		"Created",
		"Engine",
	}
	// Set all columns in the table header upfront when displaying more than one checkpoint
	if len(tasks) > 1 {
		header = append(header, "CHKPT Size", "Root Fs Diff Size")
	}

	var rows [][]string

	for _, task := range tasks {
		info, err := getCheckpointInfo(task)
		if err != nil {
			return err
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

		if len(tasks) == 1 {
			fmt.Printf("\nDisplaying container checkpoint data from %s\n\n", task.CheckpointFilePath)

			header = append(header, "CHKPT Size")
			row = append(row, metadata.ByteToString(info.archiveSizes.checkpointSize))

			// Display root fs diff size if available
			if info.archiveSizes.rootFsDiffTarSize != 0 {
				header = append(header, "Root Fs Diff Size")
				row = append(row, metadata.ByteToString(info.archiveSizes.rootFsDiffTarSize))
			}
		} else {
			row = append(row, metadata.ByteToString(info.archiveSizes.checkpointSize))
			row = append(row, metadata.ByteToString(info.archiveSizes.rootFsDiffTarSize))
		}

		rows = append(rows, row)
	}

	WriteTableHeader(w, header)
	WriteTableRows(w, rows)

	w.Flush()
	return nil
}

func getContainerInfo(specDump *spec.Spec, containerConfig *metadata.ContainerConfig, checkpointDirectory string) (*containerInfo, error) {
	var ci *containerInfo
	switch m := specDump.Annotations["io.container.manager"]; m {
	case "libpod":
		ci = getPodmanInfo(containerConfig, specDump, checkpointDirectory)
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

// archiveFileName returns the cleaned name of a tar entry if it is a file
// that checkpointctl reads: either at the top level of the archive or
// directly inside the checkpoint directory.
func archiveFileName(name string) (string, bool) {
	name = path.Clean(name)
	dir, base := path.Split(name)
	if (dir != "" && dir != metadata.CheckpointDirectory+"/") || base == "." || base == ".." {
		return "", false
	}
	return name, true
}

// matchArchiveFile reports whether name is one of the requested files.
// A requested name ending in "-" matches all files with that prefix.
func matchArchiveFile(name string, files []string) bool {
	for _, file := range files {
		if name == file || (strings.HasSuffix(file, "-") && strings.HasPrefix(name, file)) {
			return true
		}
	}
	return false
}

// extractFile writes the contents of r to name inside root.
func extractFile(root *os.Root, name string, r io.Reader) error {
	if err := root.MkdirAll(path.Dir(name), 0o700); err != nil {
		return fmt.Errorf("extracting %s: %w", name, err)
	}

	// The root keeps all file operations, including symlink
	// resolution, inside the destination directory.
	destFile, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("extracting %s: %w", name, err)
	}
	_, err = io.Copy(destFile, r)
	if closeErr := destFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("extracting %s: %w", name, err)
	}
	return nil
}

// UntarFiles unpacks the requested files from an archive to the destination
// directory. Only regular files at the top level of the archive or directly
// inside the checkpoint directory are extracted. A requested name ending in
// "-" matches all files with that prefix, such as "checkpoint/core-". Other
// entries, including absolute and nested paths and global PAX headers, are
// ignored. A requested entry that is not a regular file or a directory is an
// error. The destination directory must already exist.
func UntarFiles(src, dest string, files []string) error {
	root, err := os.OpenRoot(dest)
	if err != nil {
		return fmt.Errorf("unpacking of checkpoint archive failed: %w", err)
	}
	defer root.Close()

	if err := iterateTarArchive(src, func(r *tar.Reader, header *tar.Header) error {
		if header.Typeflag == tar.TypeXGlobalHeader {
			return nil
		}
		name, ok := archiveFileName(header.Name)
		if !ok || header.Typeflag == tar.TypeDir || !matchArchiveFile(name, files) {
			return nil
		}
		// The tar reader expands old GNU sparse files, but keeps their type.
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeCont && header.Typeflag != tar.TypeGNUSparse {
			return fmt.Errorf("%s is not a regular file", name)
		}
		return extractFile(root, name, r)
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
