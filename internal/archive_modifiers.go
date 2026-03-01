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

// remapConfigDump modifies the container runtime config JSON to update:
// - Port mappings (Podman/CRI-O: newPortMappings[].container_port)
// - PORT environment variable in any nested env arrays
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

	// Update port mappings — search for any array of objects containing
	// "container_port" (covers Podman's "newPortMappings" and
	// CRI-O's "portMappings" or any similar structure)
	remapPortMappings(config, oldPort, newPort)

	// Update PORT env var in any nested env arrays
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

// remapPortMappings recursively searches a JSON structure for arrays of objects
// containing a "container_port" key, and remaps matching ports.
// This covers Podman ("newPortMappings"), CRI-O ("portMappings"), and any
// future runtime that uses a similar convention.
func remapPortMappings(obj any, oldPort, newPort string) {
	switch v := obj.(type) {
	case map[string]any:
		for _, val := range v {
			remapPortMappings(val, oldPort, newPort)
		}
	case []any:
		for _, item := range v {
			if mapping, ok := item.(map[string]any); ok {
				if cp, ok := mapping["container_port"].(float64); ok && strconv.FormatFloat(cp, 'f', -1, 64) == oldPort {
					newPortNum, _ := strconv.ParseFloat(newPort, 64)
					mapping["container_port"] = newPortNum
				}
			}
			remapPortMappings(item, oldPort, newPort)
		}
	}
}

// remapEnvRecursive walks a JSON structure looking for any "env" key
// whose value is an array of strings, and replaces PORT=oldPort with PORT=newPort.
// This handles any runtime's config layout without hardcoding paths.
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
