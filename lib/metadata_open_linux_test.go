// SPDX-License-Identifier: Apache-2.0

//go:build linux

package metadata

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReopenFileUsesPinnedInode(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, SpecDumpFile)
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("failed to create original file: %v", err)
	}

	handle, err := os.OpenFile(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("failed to open O_PATH handle: %v", err)
	}
	defer handle.Close()

	handleInfo, err := handle.Stat()
	if err != nil {
		t.Fatalf("failed to stat O_PATH handle: %v", err)
	}

	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatalf("failed to rename original file: %v", err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatalf("failed to create replacement file: %v", err)
	}

	f, err := reopenFile(handle, path, handleInfo)
	if err != nil {
		t.Fatalf("failed to reopen O_PATH handle: %v", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read reopened file: %v", err)
	}
	if got, want := string(content), "original"; got != want {
		t.Fatalf("reopened wrong inode: got %q, want %q", got, want)
	}
}
