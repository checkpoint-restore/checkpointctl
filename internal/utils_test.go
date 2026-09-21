package internal

import (
	"archive/tar"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatTime(t *testing.T) {
	tests := []struct {
		input    uint32
		expected string
	}{
		{1, "1 µs"},
		{500, "500 µs"},
		{999, "999 µs"},
		{1001, "1.001 ms"},
		{1100, "1.1 ms"},
		{13400, "13.4 ms"},
		{1340001, "1.34 s"},
		{1340520, "1.3405 s"},
		{1340560, "1.3406 s"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Input-%d", test.input), func(t *testing.T) {
			result := FormatTime(test.input)
			if result != test.expected {
				t.Errorf("Expected %s, but got %s", test.expected, result)
			}
		})
	}
}

func writeTestArchive(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating archive: %v", err)
	}
	defer f.Close()

	tw := tar.NewWriter(f)
	defer tw.Close()

	for name, body := range entries {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(body)),
		}
		if strings.HasSuffix(name, "/") {
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0o700
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("writing header for %q: %v", name, err)
		}
		if len(body) > 0 {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatalf("writing body for %q: %v", name, err)
			}
		}
	}
}

func TestCreateTasks_CleanupOnError(t *testing.T) {
	base := t.TempDir()
	t.Setenv("TMPDIR", base)

	validArchive := filepath.Join(base, "valid.tar")
	writeTestArchive(t, validArchive, map[string]string{
		"checkpoint/": "",
		"config.dump": "config",
	})

	// Missing checkpoint directory causes CreateTasks to fail on second archive
	invalidArchive := filepath.Join(base, "invalid.tar")
	writeTestArchive(t, invalidArchive, map[string]string{
		"config.dump": "config",
	})

	tasks, err := CreateTasks([]string{validArchive, invalidArchive}, []string{"config.dump"})
	if err == nil {
		t.Fatal("expected error from CreateTasks, got nil")
	}
	if tasks != nil {
		t.Errorf("expected nil tasks on error, got %v", tasks)
	}

	entries, err := filepath.Glob(filepath.Join(base, "checkpointctl*"))
	if err != nil {
		t.Fatalf("glob after: %v", err)
	}
	if len(entries) > 0 {
		t.Errorf("leaked temporary directories found: %v", entries)
	}
}

func TestCreateTasks_CleanupOnUntarError(t *testing.T) {
	base := t.TempDir()
	t.Setenv("TMPDIR", base)

	archivePath := filepath.Join(base, "conflict.tar")
	writeTestArchive(t, archivePath, map[string]string{
		"checkpoint/":            "",
		"config.dump":            "config",
		"config.dump/child.dump": "child",
	})

	tasks, err := CreateTasks([]string{archivePath}, []string{"config.dump"})
	if err == nil {
		t.Fatal("expected UntarFiles error from file/directory path conflict, got nil")
	}
	if tasks != nil {
		t.Errorf("expected nil tasks on error, got %v", tasks)
	}

	entries, err := filepath.Glob(filepath.Join(base, "checkpointctl*"))
	if err != nil {
		t.Fatalf("glob after: %v", err)
	}
	if len(entries) > 0 {
		t.Errorf("leaked temporary directories found: %v", entries)
	}
}
