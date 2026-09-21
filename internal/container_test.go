package internal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

type tarEntry struct {
	name     string
	typeflag byte
	body     string
}

// writeArchive writes a tar or tar.gz archive containing the given entries to path.
func writeArchive(t *testing.T, path string, gzipCompress bool, entries []tarEntry) {
	t.Helper()

	var buf bytes.Buffer
	var w *tar.Writer
	var gw *gzip.Writer
	if gzipCompress {
		gw = gzip.NewWriter(&buf)
		w = tar.NewWriter(gw)
	} else {
		w = tar.NewWriter(&buf)
	}

	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Mode:     0o600,
		}
		switch e.typeflag {
		case tar.TypeDir:
			hdr.Mode = 0o700
		case tar.TypeSymlink, tar.TypeLink:
			hdr.Linkname = e.body
		default:
			hdr.Size = int64(len(e.body))
		}
		if err := w.WriteHeader(hdr); err != nil {
			t.Fatalf("writing header for %q: %v", e.name, err)
		}
		if hdr.Size > 0 {
			if _, err := w.Write([]byte(e.body)); err != nil {
				t.Fatalf("writing body for %q: %v", e.name, err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	if gw != nil {
		if err := gw.Close(); err != nil {
			t.Fatalf("closing gzip writer: %v", err)
		}
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("writing archive: %v", err)
	}
}

func TestUntarFiles(t *testing.T) {
	requiredFiles := []string{
		"config.dump",
		"spec.dump",
		filepath.Join("checkpoint", "pstree.img"),
		filepath.Join("checkpoint", "core-"),
	}

	tests := []struct {
		name        string
		gzip        bool
		entries     []tarEntry
		expectErr   bool
		extracted   []string // paths relative to dest that must exist with content
		notInDest   []string // paths relative to dest that must NOT exist
		outsideName string   // path that must NOT appear in the parent of dest
	}{
		{
			name: "valid archive",
			entries: []tarEntry{
				{"config.dump", tar.TypeReg, "config"},
				{"spec.dump", tar.TypeReg, "spec"},
				{"checkpoint/", tar.TypeDir, ""},
				{"checkpoint/pstree.img", tar.TypeReg, "pstree"},
				{"checkpoint/core-1.img", tar.TypeReg, "core1"},
				{"checkpoint/core-25.img", tar.TypeReg, "core25"},
				{"checkpoint/pages-1.img", tar.TypeReg, "pages"},
				{"rootfs-diff.tar", tar.TypeReg, "rootfs"},
			},
			extracted: []string{
				"config.dump",
				"spec.dump",
				"checkpoint/pstree.img",
				"checkpoint/core-1.img",
				"checkpoint/core-25.img",
			},
			notInDest: []string{"checkpoint/pages-1.img", "rootfs-diff.tar"},
		},
		{
			name: "valid archive with dot-slash prefix",
			entries: []tarEntry{
				{"./config.dump", tar.TypeReg, "config"},
				{"./checkpoint/", tar.TypeDir, ""},
				{"./checkpoint/core-1.img", tar.TypeReg, "core1"},
			},
			extracted: []string{"config.dump", "checkpoint/core-1.img"},
		},
		{
			name: "valid gzip archive",
			gzip: true,
			entries: []tarEntry{
				{"config.dump", tar.TypeReg, "config"},
				{"checkpoint/core-1.img", tar.TypeReg, "core1"},
			},
			extracted: []string{"config.dump", "checkpoint/core-1.img"},
		},
		{
			name: "traversal through matching prefix",
			entries: []tarEntry{
				{"config.dump/../../escape", tar.TypeReg, "escape"},
			},
			expectErr:   true,
			outsideName: "escape",
		},
		{
			name: "traversal through image prefix",
			entries: []tarEntry{
				{"checkpoint/core-../../../../escape", tar.TypeReg, "escape"},
			},
			expectErr:   true,
			outsideName: "escape",
		},
		{
			name: "internal traversal is rejected",
			entries: []tarEntry{
				{"checkpoint/core-../../../escape", tar.TypeReg, "escape"},
			},
			expectErr: true,
		},
		{
			name: "in-bound traversal attempting overwrite is rejected",
			entries: []tarEntry{
				{"checkpoint/core-x/../../spec.dump", tar.TypeReg, "evil-spec"},
			},
			expectErr: true,
		},
		{
			name: "leading dot-dot is rejected",
			entries: []tarEntry{
				{"../config.dump", tar.TypeReg, "escape"},
			},
			expectErr: true,
		},
		{
			name: "directory entry with traversal name",
			entries: []tarEntry{
				{"config.dump/../../escapedir/", tar.TypeDir, ""},
			},
			// Not a regular file: skipped before any matching.
			outsideName: "escapedir",
		},
		{
			name: "absolute path is rejected",
			entries: []tarEntry{
				{"/tmp/config.dump", tar.TypeReg, "escape"},
			},
			expectErr: true,
		},
		{
			name: "misleading substring name",
			entries: []tarEntry{
				{"misleading-config.dump.backup", tar.TypeReg, "decoy"},
			},
			notInDest: []string{"misleading-config.dump.backup", "config.dump"},
		},
		{
			name: "exact filename does not match suffix decoys",
			entries: []tarEntry{
				{"config.dump.backup", tar.TypeReg, "decoy"},
				{"spec.dump.json", tar.TypeReg, "decoy"},
			},
			notInDest: []string{"config.dump.backup", "config.dump", "spec.dump.json", "spec.dump"},
		},
		{
			name: "directory collision with exact filename is skipped",
			entries: []tarEntry{
				{"config.dump/subfile", tar.TypeReg, "collision"},
			},
			notInDest: []string{"config.dump/subfile", "config.dump"},
		},
		{
			name: "directory collision with image prefix is skipped",
			entries: []tarEntry{
				{"checkpoint/core-1.img/subfile", tar.TypeReg, "collision"},
			},
			notInDest: []string{"checkpoint/core-1.img/subfile", "checkpoint/core-1.img"},
		},
		{
			name: "backslash traversal is rejected",
			entries: []tarEntry{
				{`checkpoint\core-..\..\escape`, tar.TypeReg, "escape"},
			},
			expectErr:   true,
			outsideName: "escape",
		},
		{
			name: "nested decoy",
			entries: []tarEntry{
				{"evil/config.dump", tar.TypeReg, "decoy"},
				{"evil/checkpoint/core-1.img", tar.TypeReg, "decoy"},
			},
			notInDest: []string{"evil/config.dump", "evil/checkpoint/core-1.img", "config.dump"},
		},
		{
			name: "symlink entry is skipped",
			entries: []tarEntry{
				{"config.dump", tar.TypeSymlink, "/etc/passwd"},
			},
			notInDest: []string{"config.dump"},
		},
		{
			name: "hardlink entry is skipped",
			entries: []tarEntry{
				{"checkpoint/core-1.img", tar.TypeLink, "checkpoint/pages-1.img"},
			},
			notInDest: []string{"checkpoint/core-1.img"},
		},
		{
			name: "duplicate entries last wins",
			entries: []tarEntry{
				{"config.dump", tar.TypeReg, "first"},
				{"config.dump", tar.TypeReg, "second"},
			},
			extracted: []string{"config.dump"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			dest := filepath.Join(base, "dest")
			if err := os.MkdirAll(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			archiveName := "checkpoint.tar"
			if test.gzip {
				archiveName = "checkpoint.tar.gz"
			}
			archivePath := filepath.Join(base, archiveName)
			writeArchive(t, archivePath, test.gzip, test.entries)

			err := UntarFiles(archivePath, dest, requiredFiles)
			if test.expectErr && err == nil {
				t.Errorf("expected an error, got nil")
			}
			if !test.expectErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			for _, rel := range test.extracted {
				content, statErr := os.ReadFile(filepath.Join(dest, rel))
				if statErr != nil {
					t.Errorf("expected %q to be extracted: %v", rel, statErr)
					continue
				}
				if len(content) == 0 {
					t.Errorf("expected %q to have content", rel)
				}
			}
			for _, rel := range test.notInDest {
				if _, statErr := os.Lstat(filepath.Join(dest, rel)); statErr == nil {
					t.Errorf("expected %q to NOT be extracted", rel)
				}
			}
			if test.outsideName != "" {
				if _, statErr := os.Lstat(filepath.Join(base, test.outsideName)); statErr == nil {
					t.Errorf("expected %q to NOT be created outside the destination", test.outsideName)
				}
			}
		})
	}
}
