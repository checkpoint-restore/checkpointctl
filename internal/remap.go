// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/checkpoint-restore/go-criu/v8/crit"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/fdinfo"
	"github.com/containers/storage/pkg/archive"
)

const (
	// TCP protocol number (IPPROTO_TCP)
	IPPROTO_TCP = 6
	// SOCK_STREAM socket type
	SOCK_STREAM = 1
	// TCP_LISTEN state (see linux/net/tcp_states.h)
	TCP_LISTEN = 10
)

// RemapTCPListenPorts remaps TCP listen ports in files.img according to the provided mappings.
// Returns true if any ports were remapped, false otherwise.
func RemapTCPListenPorts(filesImgPath string, portMappings map[uint32]uint32) (bool, error) {
	// Open the files.img file for reading
	file, err := os.Open(filesImgPath)
	if err != nil {
		return false, fmt.Errorf("failed to open files.img: %w", err)
	}
	defer file.Close()

	// Decode the files.img
	c := crit.New(file, nil, "", false, false)
	img, err := c.Decode(&fdinfo.FileEntry{})
	if err != nil {
		return false, fmt.Errorf("failed to decode files.img: %w", err)
	}

	modified := false

	// Iterate through all entries in files.img
	for _, entry := range img.Entries {
		fileEntry, ok := entry.Message.(*fdinfo.FileEntry)
		if !ok {
			continue
		}

		// Check if this is an INETSK entry
		if fileEntry.GetType() != fdinfo.FdTypes_INETSK {
			continue
		}

		inetSk := fileEntry.GetIsk()
		if inetSk == nil {
			continue
		}

		// Check if this is a TCP SOCK_STREAM socket in LISTEN state
		if inetSk.GetProto() != IPPROTO_TCP || inetSk.GetType() != SOCK_STREAM || inetSk.GetState() != TCP_LISTEN {
			continue
		}

		// Get the current source port
		srcPort := inetSk.GetSrcPort()
		if srcPort == 0 {
			continue
		}

		// Check if this port needs to be remapped
		newPort, needsRemap := portMappings[srcPort]
		if !needsRemap {
			continue
		}

		// Remap the port
		inetSk.SrcPort = &newPort
		modified = true
	}

	if !modified {
		return false, nil
	}

	// Close the input file before creating output file
	file.Close()

	// Create a temporary file for output
	tempOutputPath := filesImgPath + ".tmp"
	outputFile, err := os.Create(tempOutputPath)
	if err != nil {
		return false, fmt.Errorf("failed to create temporary output file: %w", err)
	}
	defer outputFile.Close()

	// Encode the modified image to the temporary file
	cOut := crit.New(nil, outputFile, "", false, false)
	if err := cOut.Encode(img); err != nil {
		outputFile.Close()
		os.Remove(tempOutputPath)
		return false, fmt.Errorf("failed to encode modified files.img: %w", err)
	}

	outputFile.Close()

	// Replace the original file with the modified one
	if err := os.Rename(tempOutputPath, filesImgPath); err != nil {
		os.Remove(tempOutputPath)
		return false, fmt.Errorf("failed to replace files.img: %w", err)
	}

	return true, nil
}

// RepackFileToArchive repacks a modified file back into the checkpoint archive.
func RepackFileToArchive(checkpointPath, filePath string, modifiedFilePath string) error {
	// This is a simplified implementation that replaces the file in the archive
	// In a production implementation, we'd want to:
	// 1. Extract the entire archive
	// 2. Replace the modified file
	// 3. Recompress and repack the archive

	// For now, we'll use a simple approach: replace the file directly in the archive
	// This works if the new file is not larger than the original

	// Open the checkpoint archive for reading
	archiveFile, err := os.Open(checkpointPath)
	if err != nil {
		return fmt.Errorf("failed to open checkpoint archive: %w", err)
	}
	defer archiveFile.Close()

	// Read the modified file
	modifiedFile, err := os.ReadFile(modifiedFilePath)
	if err != nil {
		return fmt.Errorf("failed to read modified file: %w", err)
	}

	// Create a temporary archive with the modified file
	tempArchivePath := checkpointPath + ".tmp"
	if err := repackArchiveWithFile(archiveFile, tempArchivePath, filePath, modifiedFile); err != nil {
		os.Remove(tempArchivePath)
		return fmt.Errorf("failed to repack archive: %w", err)
	}

	// Replace the original archive with the new one
	if err := os.Rename(tempArchivePath, checkpointPath); err != nil {
		os.Remove(tempArchivePath)
		return fmt.Errorf("failed to replace checkpoint archive: %w", err)
	}

	return nil
}

// repackArchiveWithFile creates a new archive with a replaced file
func repackArchiveWithFile(archiveFile *os.File, outputPath, fileToReplace string, newFileContent []byte) error {
	// This is a placeholder implementation
	// In production, we'd need to:
	// 1. Decompress the archive
	// 2. Iterate through entries, replacing the target file
	// 3. Recompress and write to output

	// For now, we'll use the archive package to help with this
	// Since this is complex, let's implement a simpler version first

	// We need to extract the archive, replace the file, and repack it
	// This is the most reliable approach

	tempDir, err := os.MkdirTemp("", "checkpointctl-repack-")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract entire archive
	if err := extractArchive(archiveFile.Name(), tempDir); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	// Replace the file
	destPath := filepath.Join(tempDir, fileToReplace)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(destPath, newFileContent, 0o644); err != nil {
		return fmt.Errorf("failed to write modified file: %w", err)
	}

	// Repack the archive
	if err := packDirectory(tempDir, outputPath); err != nil {
		return fmt.Errorf("failed to pack archive: %w", err)
	}

	return nil
}

// extractArchive extracts an entire archive to a directory
func extractArchive(archivePath, destDir string) error {
	archiveFile, err := os.Open(archivePath)
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

		targetPath := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
				return err
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// packDirectory creates an archive from a directory
func packDirectory(srcDir, archivePath string) error {
	// Use the same compression format as the original
	// For simplicity, we'll use gzip like most checkpoint archives
	return packDirectoryTarGz(srcDir, archivePath)
}

// packDirectoryTarGz creates a tar.gz archive from a directory
func packDirectoryTarGz(srcDir, archivePath string) error {
	outputFile, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	// Create gzip writer
	gzWriter := gzip.NewWriter(outputFile)
	defer gzWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Use forward slashes for tar entries (POSIX format)
		relPath = filepath.ToSlash(relPath)

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		header.Name = relPath
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(tarWriter, file)
			return err
		}

		return nil
	})
}