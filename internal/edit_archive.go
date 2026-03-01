// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"archive/tar"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/containers/storage/pkg/archive"
)

// TcpListenRemap remaps the TCP listen ports specified by the portMapping
// of a checkpoint archive specified by the archivePath and replaces the old
// archive with the new one.
// portMapping uses the format old-port:new-port
func TcpListenRemap(portMapping, archivePath string) error {
	ports := strings.SplitN(portMapping, ":", 2)
	if len(ports) != 2 {
		return fmt.Errorf("invalid parameters: expected oldport:newport")
	}

	// Parse ports
	oldPort, err := strconv.ParseUint(ports[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid parameters: expected oldport:newport")
	}
	if oldPort == 0 || oldPort > 65535 {
		return fmt.Errorf("old port %d is out of valid range (1-65535)", oldPort)
	}

	newPort, err := strconv.ParseUint(ports[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid parameters: expected oldport:newport")
	}
	if newPort == 0 || newPort > 65535 {
		return fmt.Errorf("new port %d is out of valid range (1-65535)", newPort)
	}

	oldPortStr := strconv.FormatUint(oldPort, 10)
	newPortStr := strconv.FormatUint(newPort, 10)

	// Define modifiers for tar entries that need port remapping
	mods := map[string]func(*tar.Header, io.Reader) (*tar.Header, []byte, error){
		// Remap the TCP listen port in the CRIU binary image
		"checkpoint/files.img": func(hdr *tar.Header, content io.Reader) (*tar.Header, []byte, error) {
			return remapFilesImg(hdr, content, uint32(oldPort), uint32(newPort))
		},
		// Remap port mappings and PORT env var in the container config
		"config.dump": func(hdr *tar.Header, content io.Reader) (*tar.Header, []byte, error) {
			return remapConfigDump(hdr, content, oldPortStr, newPortStr)
		},
		// Remap PORT env var in the OCI runtime spec
		"spec.dump": func(hdr *tar.Header, content io.Reader) (*tar.Header, []byte, error) {
			return remapSpecDump(hdr, content, oldPortStr, newPortStr)
		},
	}

	if err := tarStreamRewrite(archivePath, mods); err != nil {
		return err
	}

	log.Printf("Successfully remapped port %d -> %d\n", oldPort, newPort)

	return nil
}

// tarStreamRewrite streams through a (possibly compressed) tar archive at archivePath,
// applies modifications to entries as specified by the mods map, writes the result to a temporary file,
// and atomically replaces the original archive with the modified one.
// The compression type and file permissions of the original archive are preserved.
func tarStreamRewrite(archivePath string, mods map[string]func(*tar.Header, io.Reader) (*tar.Header, []byte, error)) error {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	// Check if there is a checkpoint directory in the archive file
	checkpointDirExists, err := isFileInArchive(archivePath, metadata.CheckpointDirectory, true)
	if err != nil {
		return err
	}
	if !checkpointDirExists {
		return fmt.Errorf("checkpoint directory is missing in the archive file: %s", archivePath)
	}

	// For getting input archive's permissions later
	archiveInfo, err := archiveFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat archive: %w", err)
	}

	// Detect Compression
	b := make([]byte, 10)
	n, err := io.ReadFull(archiveFile, b)
	if err != nil {
		return fmt.Errorf("failed to read archive magic bytes")
	}
	comp := archive.DetectCompression(b[:n])
	// Seek back so DecompressStream can read from the start
	if _, err := archiveFile.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek archive")
	}

	// Decompress the archive into a plan tar stream
	tarStream, err := archive.DecompressStream(archiveFile)
	if err != nil {
		return fmt.Errorf("failed to decompress archive")
	}
	defer tarStream.Close()

	// Create output file with compression
	outFile, err := os.CreateTemp(filepath.Dir(archivePath), ".checkpointctl-edit-*.tar")
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	outputPath := outFile.Name()

	compressor, err := archive.CompressStream(outFile, comp)
	if err != nil {
		outFile.Close()
		os.Remove(outputPath)
		return fmt.Errorf("failed to create compressor")
	}

	tarReader := tar.NewReader(tarStream)
	tarWriter := tar.NewWriter(compressor)
	matchedEntries := 0

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			tarWriter.Close()
			compressor.Close()
			outFile.Close()
			os.Remove(outputPath)
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		entryName := normalizeArchivePath(header.Name)

		// Check if this entry has a modifier
		if modifier, ok := mods[entryName]; ok {
			matchedEntries++
			newHeader, data, err := modifier(header, tarReader)
			if err != nil {
				tarWriter.Close()
				compressor.Close()
				outFile.Close()
				os.Remove(outputPath)
				return err
			}
			if newHeader != nil {
				newHeader.Size = int64(len(data))
				if err := tarWriter.WriteHeader(newHeader); err != nil {
					tarWriter.Close()
					compressor.Close()
					outFile.Close()
					os.Remove(outputPath)
					return fmt.Errorf("failed to write header for %s: %w", header.Name, err)
				}
				if _, err := tarWriter.Write(data); err != nil {
					tarWriter.Close()
					compressor.Close()
					outFile.Close()
					os.Remove(outputPath)
					return fmt.Errorf("failed to write data for %s: %w", header.Name, err)
				}
			}
		} else {
			// Copy entry unchanged
			if err := tarWriter.WriteHeader(header); err != nil {
				tarWriter.Close()
				compressor.Close()
				outFile.Close()
				os.Remove(outputPath)
				return fmt.Errorf("failed to write header for %s: %w", header.Name, err)
			}
			if _, err := io.Copy(tarWriter, tarReader); err != nil {
				tarWriter.Close()
				compressor.Close()
				outFile.Close()
				os.Remove(outputPath)
				return fmt.Errorf("failed to copy data for %s: %w", header.Name, err)
			}
		}
	}

	if err := tarWriter.Close(); err != nil {
		compressor.Close()
		outFile.Close()
		os.Remove(outputPath)
		return fmt.Errorf("failed to finalize tar stream: %w", err)
	}
	if err := compressor.Close(); err != nil {
		outFile.Close()
		os.Remove(outputPath)
		return fmt.Errorf("failed to finalize compressed stream: %w", err)
	}
	if err := outFile.Close(); err != nil {
		os.Remove(outputPath)
		return fmt.Errorf("failed to close output file: %w", err)
	}
	if matchedEntries != len(mods) {
		os.Remove(outputPath)
		return fmt.Errorf("matching entries not found in archive for requested edit operation")
	}

	// Match the output file's permissions to the input archive
	if err := os.Chmod(outputPath, archiveInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("failed to set output permissions: %w", err)
	}

	// Replace the modified archive with the original one
	if err := os.Rename(outputPath, archivePath); err != nil {
		os.Remove(outputPath)
		return fmt.Errorf("failed to replace checkpoint: %w", err)
	}

	return nil
}

func normalizeArchivePath(path string) string {
	return strings.TrimPrefix(path, "./")
}
