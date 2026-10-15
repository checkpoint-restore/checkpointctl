package internal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		plantLink   string // create dest/<plantLink> as a symlink to a dir outside dest
		expectErr   bool
		extracted   map[string]string // paths relative to dest -> expected content
		notInDest   []string          // paths relative to dest that must NOT exist
		outsideName string            // path relative to base that must NOT appear outside dest
		errContains string            // substring the error must contain, if any
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
			extracted: map[string]string{
				"config.dump":            "config",
				"spec.dump":              "spec",
				"checkpoint/pstree.img":  "pstree",
				"checkpoint/core-1.img":  "core1",
				"checkpoint/core-25.img": "core25",
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
			extracted: map[string]string{
				"config.dump":           "config",
				"checkpoint/core-1.img": "core1",
			},
		},
		{
			name: "valid gzip archive",
			gzip: true,
			entries: []tarEntry{
				{"config.dump", tar.TypeReg, "config"},
				{"checkpoint/core-1.img", tar.TypeReg, "core1"},
			},
			extracted: map[string]string{
				"config.dump":           "config",
				"checkpoint/core-1.img": "core1",
			},
		},

		{
			name: "contiguous file is extracted",
			entries: []tarEntry{
				{"checkpoint/core-1.img", tar.TypeCont, "contiguous-core"},
			},
			extracted: map[string]string{
				"checkpoint/core-1.img": "contiguous-core",
			},
		},
		{
			name: "unmatched entries with traversal or absolute paths are skipped",
			entries: []tarEntry{
				{"config.dump/../../escape", tar.TypeReg, "escape"},
				{"checkpoint/core-../../../../escape", tar.TypeReg, "escape"},
				{"checkpoint/core-../../../escape", tar.TypeReg, "escape"},
				{"../config.dump", tar.TypeReg, "escape"},
				{"/tmp/config.dump", tar.TypeReg, "escape"},
				{`checkpoint\core-..\..\escape`, tar.TypeReg, "escape"},
				{"config.dump", tar.TypeReg, "config"},
			},
			extracted: map[string]string{
				"config.dump": "config",
			},
			outsideName: "escape",
		},
		{
			name: "in-bound traversal attempting overwrite is rejected",
			entries: []tarEntry{
				{"checkpoint/core-x/../../spec.dump", tar.TypeReg, "evil-spec"},
			},
			expectErr:   true,
			errContains: "contains relative traversal element",
		},
		{
			name: "relative traversal matching target file is rejected",
			entries: []tarEntry{
				{"checkpoint/../config.dump", tar.TypeReg, "evil"},
			},
			expectErr:   true,
			errContains: "contains relative traversal element",
		},
		{
			name: "prefix match with traversal element is rejected",
			entries: []tarEntry{
				{"checkpoint/sub/../core-1.img", tar.TypeReg, "evil"},
			},
			expectErr:   true,
			errContains: "contains relative traversal element",
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
			name: "unmatched child of extracted regular file is skipped",
			entries: []tarEntry{
				{"config.dump", tar.TypeReg, "config"},
				{"config.dump/child.dump", tar.TypeReg, "child"},
			},
			extracted: map[string]string{
				"config.dump": "config",
			},
			notInDest: []string{"config.dump/child.dump"},
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
			name: "symlink entry matching a requested file is rejected",
			entries: []tarEntry{
				{"config.dump", tar.TypeSymlink, "/etc/passwd"},
			},
			expectErr:   true,
			errContains: "unsupported type",
			notInDest:   []string{"config.dump"},
		},
		{
			name: "hardlink entry matching a requested file is rejected",
			entries: []tarEntry{
				{"checkpoint/core-1.img", tar.TypeLink, "checkpoint/pages-1.img"},
			},
			expectErr:   true,
			errContains: "unsupported type",
			notInDest:   []string{"checkpoint/core-1.img"},
		},
		{
			name:      "planted symlink in destination is not followed",
			plantLink: "checkpoint",
			entries: []tarEntry{
				{"checkpoint/pstree.img", tar.TypeReg, "pwned"},
			},
			expectErr:   true,
			errContains: "path escapes from parent",
			outsideName: "outside/pstree.img",
		},
		{
			name: "duplicate entries last wins",
			entries: []tarEntry{
				{"config.dump", tar.TypeReg, "first"},
				{"config.dump", tar.TypeReg, "second"},
			},
			extracted: map[string]string{
				"config.dump": "second",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			dest := filepath.Join(base, "dest")
			if test.plantLink != "" {
				if err := os.MkdirAll(dest, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			archiveName := "checkpoint.tar"
			if test.gzip {
				archiveName = "checkpoint.tar.gz"
			}
			archivePath := filepath.Join(base, archiveName)
			writeArchive(t, archivePath, test.gzip, test.entries)

			if test.plantLink != "" {
				// dest/<plantLink> points outside dest, so writing through it
				// would escape the extraction directory.
				outside := filepath.Join(base, "outside")
				if mkErr := os.MkdirAll(outside, 0o700); mkErr != nil {
					t.Fatal(mkErr)
				}
				if lnErr := os.Symlink(outside, filepath.Join(dest, test.plantLink)); lnErr != nil {
					t.Fatal(lnErr)
				}
			}

			err := UntarFiles(archivePath, dest, requiredFiles)
			if test.expectErr && err == nil {
				t.Errorf("expected an error, got nil")
			}
			if !test.expectErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
			if test.errContains != "" && err != nil && !strings.Contains(err.Error(), test.errContains) {
				t.Errorf("expected error containing %q, got %v", test.errContains, err)
			}

			for rel, want := range test.extracted {
				content, statErr := os.ReadFile(filepath.Join(dest, rel))
				if statErr != nil {
					t.Errorf("expected %q to be extracted: %v", rel, statErr)
					continue
				}
				if string(content) != want {
					t.Errorf("expected %q content %q, got %q", rel, want, string(content))
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

func TestUntarFiles_GNUSparseArchive(t *testing.T) {
	tarPath, err := exec.LookPath("tar")
	if err != nil {
		t.Skip("tar executable not found")
	}

	base := t.TempDir()
	srcDir := filepath.Join(base, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "checkpoint"), 0o700); err != nil {
		t.Fatal(err)
	}

	sparseFile := filepath.Join(srcDir, "checkpoint", "core-1.img")
	f, err := os.Create(sparseFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("sparse-data")); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1024 * 1024); err != nil {
		t.Fatal(err)
	}
	f.Close()

	archivePath := filepath.Join(base, "checkpoint.tar")
	cmd := exec.Command(tarPath, "-Scf", archivePath, "-C", srcDir, "checkpoint/core-1.img")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating sparse tar archive: %v, output: %s", err, string(out))
	}

	dest := filepath.Join(base, "dest")
	if err := UntarFiles(archivePath, dest, []string{"checkpoint/core-"}); err != nil {
		t.Fatalf("extracting sparse archive failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dest, "checkpoint", "core-1.img"))
	if err != nil {
		t.Fatalf("reading extracted sparse file: %v", err)
	}
	if len(data) != 1024*1024 || !strings.HasPrefix(string(data), "sparse-data") {
		t.Fatalf("unexpected sparse file contents length: %d", len(data))
	}
}

func TestUntarFiles_NonExistentDest(t *testing.T) {
	base := t.TempDir()
	archivePath := filepath.Join(base, "archive.tar")
	writeArchive(t, archivePath, false, []tarEntry{
		{"config.dump", tar.TypeReg, "config"},
	})

	dest := filepath.Join(base, "nested", "dest", "dir")
	if err := UntarFiles(archivePath, dest, []string{"config.dump"}); err != nil {
		t.Fatalf("UntarFiles failed with non-existent dest: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dest, "config.dump"))
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(content) != "config" {
		t.Fatalf("expected 'config', got %q", string(content))
	}
}
