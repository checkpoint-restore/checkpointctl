// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/checkpoint-restore/go-criu/v8/crit"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/fdinfo"
)

// TCP_LISTEN state value from the Linux kernel
const tcpListenState = 10

// remapFilesImg decodes a CRIU files.img binary image, remaps the source port
// of any TCP listen socket matching oldPort to newPort, and re-encodes the image.
func remapFilesImg(hdr *tar.Header, content io.Reader, oldPort, newPort uint32) (*tar.Header, []byte, error) {
	// crit.New requires *os.File, so write the tar entry content to a temp file
	tmpIn, err := os.CreateTemp("", "files-img-in-*.img")
	if err != nil {
		return nil, nil, fmt.Errorf("creating temp input file: %w", err)
	}
	defer os.Remove(tmpIn.Name())
	defer tmpIn.Close()

	if _, err := io.Copy(tmpIn, content); err != nil {
		return nil, nil, fmt.Errorf("writing to temp file: %w", err)
	}
	if _, err := tmpIn.Seek(0, 0); err != nil {
		return nil, nil, fmt.Errorf("seeking temp file: %w", err)
	}

	// Decode the binary image
	c := crit.New(tmpIn, nil, "", false, false)
	img, err := c.Decode(&fdinfo.FileEntry{})
	if err != nil {
		return nil, nil, fmt.Errorf("decoding files.img: %w", err)
	}

	// Walk every entry looking for TCP listen sockets on the old port
	remapped := 0
	for _, entry := range img.Entries {
		fileEntry, ok := entry.Message.(*fdinfo.FileEntry)
		if !ok {
			continue
		}
		if fileEntry.GetType() != fdinfo.FdTypes_INETSK {
			continue
		}
		isk := fileEntry.GetIsk()
		if isk == nil {
			continue
		}
		if isk.GetState() == tcpListenState && isk.GetSrcPort() == oldPort {
			np := newPort
			isk.SrcPort = &np
			remapped++
		}
	}

	if remapped == 0 {
		return nil, nil, fmt.Errorf("no TCP listen sockets found with source port %d", oldPort)
	}

	// Encode the modified image to another temp file
	tmpOut, err := os.CreateTemp("", "files-img-out-*.img")
	if err != nil {
		return nil, nil, fmt.Errorf("creating temp output file: %w", err)
	}
	defer os.Remove(tmpOut.Name())
	defer tmpOut.Close()

	cOut := crit.New(nil, tmpOut, "", false, false)
	if err := cOut.Encode(img); err != nil {
		return nil, nil, fmt.Errorf("encoding files.img: %w", err)
	}

	// Read the re-encoded bytes
	if _, err := tmpOut.Seek(0, 0); err != nil {
		return nil, nil, fmt.Errorf("seeking output file: %w", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, tmpOut); err != nil {
		return nil, nil, fmt.Errorf("reading output file: %w", err)
	}

	// Update the tar header to reflect the new size
	hdr.Size = int64(buf.Len())
	return hdr, buf.Bytes(), nil
}

// remapConfigDump modifies the config dump in a Podman checkpoint to update:
// - Port mappings
// - PORT environment variable in any nested env arrays
// Returns silently for other runtime checkpoints.
func remapConfigDump(hdr *tar.Header, content io.Reader, oldPort, newPort string) (*tar.Header, []byte, error) {
	data, err := io.ReadAll(content)
	if err != nil {
		return nil, nil, fmt.Errorf("reading config.dump: %w", err)
	}

	// Parse into a generic map to preserve all fields
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, nil, fmt.Errorf("parsing config.dump JSON: %w", err)
	}

	remapPortMappings(config, oldPort, newPort)

	remapEnvRecursive(config, oldPort, newPort)

	output, err := json.Marshal(config)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling config.dump: %w", err)
	}

	hdr.Size = int64(len(output))
	return hdr, output, nil
}

// remapSpecDump modifies the OCI runtime spec JSON to update the PORT env var.
func remapSpecDump(hdr *tar.Header, content io.Reader, oldPort, newPort string) (*tar.Header, []byte, error) {
	data, err := io.ReadAll(content)
	if err != nil {
		return nil, nil, fmt.Errorf("reading spec.dump: %w", err)
	}

	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, nil, fmt.Errorf("parsing spec.dump JSON: %w", err)
	}

	// The env array lives under spec.process.env
	if process, ok := spec["process"].(map[string]any); ok {
		if envSlice, ok := process["env"].([]any); ok {
			process["env"] = remapEnvSlice(envSlice, oldPort, newPort)
		}
	}

	output, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling spec.dump: %w", err)
	}

	hdr.Size = int64(len(output))
	return hdr, output, nil
}

// remapPortMappings updates the container_port field in the objects of
// newPortMappings array in obj. It searches for port mappings where
// container_port matches oldPort and replaces them with newPort.
func remapPortMappings(obj any, oldPort, newPort string) {
	m, ok := obj.(map[string]any)
	if !ok {
		return
	}

	mappings, ok := m["newPortMappings"]
	if !ok {
		return
	}

	mappingsSlice, ok := mappings.([]any)
	if !ok {
		return
	}

	for _, mapping := range mappingsSlice {
		mappingMap, ok := mapping.(map[string]any)
		if !ok {
			continue
		}

		containerPort, ok := mappingMap["container_port"]
		if !ok {
			continue
		}

		// JSON numbers are unmarshaled as float64
		portFloat, ok := containerPort.(float64)
		if !ok {
			continue
		}

		if strconv.FormatFloat(portFloat, 'f', -1, 64) == oldPort {
			newPortNum, _ := strconv.ParseFloat(newPort, 64)
			mappingMap["container_port"] = newPortNum
		}
	}
}

// remapEnvRecursive walks the structure obj looking for any "env" key
// whose value is an array of strings, and replaces PORT=oldPort with PORT=newPort.
func remapEnvRecursive(obj any, oldPort, newPort string) {
	m, ok := obj.(map[string]any)
	if !ok {
		return
	}
	for key, val := range m {
		if key == "env" {
			if envSlice, ok := val.([]any); ok {
				m["env"] = remapEnvSlice(envSlice, oldPort, newPort)
			}
		} else {
			switch child := val.(type) {
			case map[string]any:
				remapEnvRecursive(child, oldPort, newPort)
			case []any:
				for _, item := range child {
					remapEnvRecursive(item, oldPort, newPort)
				}
			}
		}
	}
}

// remapEnvSlice replaces PORT=oldPort with PORT=newPort in an env slice.
func remapEnvSlice(envSlice []any, oldPort, newPort string) []any {
	target := "PORT=" + oldPort
	replacement := "PORT=" + newPort
	for i, v := range envSlice {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if s == target {
			envSlice[i] = replacement
		}
	}
	return envSlice
}
