// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"archive/tar"
	"os"
	"sort"
	"testing"
)

func writeCheckpointArchive(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	}()

	// Some tests create archives as root and read them as an unprivileged user.
	if err := file.Chmod(0o644); err != nil {
		t.Fatal(err)
	}

	archive := tar.NewWriter(file)
	directory := &tar.Header{
		Name:     "checkpoint/",
		Typeflag: tar.TypeDir,
		Mode:     0o700,
	}
	if err := archive.WriteHeader(directory); err != nil {
		t.Fatal(err)
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		data := files[name]
		header := &tar.Header{
			Name: name,
			Size: int64(len(data)),
			Mode: 0o600,
		}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatalf("writing header for %s: %v", name, err)
		}
		if _, err := archive.Write(data); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}
