package internal

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type testTarEntry struct {
	name     string
	typeflag byte
	linkname string
	body     string
	pax      map[string]string
}

func regular(name, body string) testTarEntry {
	return testTarEntry{name: name, typeflag: tar.TypeReg, body: body}
}

func symlink(name, target string) testTarEntry {
	return testTarEntry{name: name, typeflag: tar.TypeSymlink, linkname: target}
}

func hardlink(name, target string) testTarEntry {
	return testTarEntry{name: name, typeflag: tar.TypeLink, linkname: target}
}

func special(name string, typeflag byte) testTarEntry {
	return testTarEntry{name: name, typeflag: typeflag}
}

func writeOrderedTestArchive(t *testing.T, archivePath string, entries []testTarEntry) {
	t.Helper()
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("creating archive: %v", err)
	}
	defer f.Close()

	tw := tar.NewWriter(f)
	for _, entry := range entries {
		hdr := &tar.Header{
			Name:       entry.name,
			Typeflag:   entry.typeflag,
			Linkname:   entry.linkname,
			Mode:       0o644,
			Size:       int64(len(entry.body)),
			PAXRecords: entry.pax,
		}
		if entry.typeflag == tar.TypeXGlobalHeader {
			hdr.Mode = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("writing header for %q: %v", entry.name, err)
		}
		if _, err := tw.Write([]byte(entry.body)); err != nil {
			t.Fatalf("writing body for %q: %v", entry.name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing archive: %v", err)
	}
}

func TestUntarFilesCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		entries []testTarEntry
	}{
		{
			name: "contiguous file",
			entries: []testTarEntry{
				{name: "config.dump", typeflag: tar.TypeCont, body: "config"},
			},
		},
		{
			name: "global PAX header matching requested file",
			entries: []testTarEntry{
				{name: "config.dump", typeflag: tar.TypeXGlobalHeader, pax: map[string]string{"comment": "global metadata"}},
				regular("config.dump", "config"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			dest := filepath.Join(base, "dest")
			if err := os.Mkdir(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			archivePath := filepath.Join(base, "archive.tar")
			writeOrderedTestArchive(t, archivePath, tt.entries)
			if err := UntarFiles(archivePath, dest, []string{"config.dump"}); err != nil {
				t.Fatalf("UntarFiles failed: %v", err)
			}
			content, err := os.ReadFile(filepath.Join(dest, "config.dump"))
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != "config" {
				t.Errorf("extracted content = %q, want %q", content, "config")
			}
		})
	}
}

// listFiles returns the slash-separated paths of all files under dir, and
// fails the test if any of them is not a regular file.
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			t.Errorf("extracted non-regular file %s (%s)", p, d.Type())
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return files
}

func TestUntarFiles(t *testing.T) {
	// The destination is base/dest. Entries escape into base/outside, so that
	// any escape is detected without writing outside t.TempDir(). absOutside
	// is replaced with the absolute path of base/outside.
	const absOutside = "{{outside}}"

	tests := []struct {
		name    string
		entries []testTarEntry
		files   []string
		want    []string
		wantErr string
	}{
		{
			name: "requested files and prefixes",
			entries: []testTarEntry{
				special("./", tar.TypeDir),
				special("./checkpoint/", tar.TypeDir),
				regular("./spec.dump", "spec"),
				regular("./config.dump", "config"),
				regular("./checkpoint/core-1.img", "core"),
				regular("./checkpoint/pstree.img", "pstree"),
			},
			files: []string{"spec.dump", "checkpoint/core-"},
			want:  []string{"checkpoint/core-1.img", "spec.dump"},
		},
		{
			name: "names are cleaned",
			entries: []testTarEntry{
				regular("checkpoint//./core-1.img", "core"),
				regular("checkpoint/../spec.dump", "spec"),
			},
			files: []string{"spec.dump", "checkpoint/core-"},
			want:  []string{"checkpoint/core-1.img", "spec.dump"},
		},
		{
			name: "names containing dots",
			entries: []testTarEntry{
				regular("notes..txt", "notes"),
				regular("checkpoint/core-1..img", "core"),
			},
			files: []string{"checkpoint/core-"},
			want:  []string{"checkpoint/core-1..img"},
		},
		{
			name: "prefix boundaries",
			entries: []testTarEntry{
				regular("other-config.dump", "config"),
				regular("checkpoint-extra/core-1.img", "core"),
				regular("checkpoint/amdgpu-pages-1.img", "amdgpu"),
				regular("checkpoint/pages-1.img", "pages"),
			},
			files: []string{"config.dump", "checkpoint/core-", "checkpoint/pages-"},
			want:  []string{"checkpoint/pages-1.img"},
		},
		{
			name: "nested files are ignored",
			entries: []testTarEntry{
				regular("spec.dump/child", "child"),
				regular("checkpoint/sub/core-1.img", "core"),
			},
			files: []string{"spec.dump", "checkpoint/core-"},
		},
		{
			name: "absolute names are ignored",
			entries: []testTarEntry{
				regular("/spec.dump", "spec"),
				regular("/checkpoint/core-1.img", "core"),
			},
			files: []string{"spec.dump", "checkpoint/core-"},
		},
		{
			name: "parent traversal",
			entries: []testTarEntry{
				regular("../outside/config.dump", "evil"),
				regular("checkpoint/../../outside/spec.dump", "evil"),
				regular("..", "evil"),
			},
			files: []string{"config.dump", "spec.dump", ".."},
		},
		{
			name: "absolute path outside the destination",
			entries: []testTarEntry{
				regular(absOutside+"/spec.dump", "evil"),
			},
			files: []string{"spec.dump"},
		},
		{
			name: "symlinked checkpoint directory",
			entries: []testTarEntry{
				symlink("checkpoint", "../outside"),
				regular("checkpoint/core-1.img", "core"),
			},
			files: []string{"checkpoint/core-"},
			want:  []string{"checkpoint/core-1.img"},
		},
		{
			name: "unrequested links and special files are ignored",
			entries: []testTarEntry{
				symlink("link", "../outside/link"),
				hardlink("checkpoint/pstree.img", "/etc/passwd"),
				special("checkpoint/fifo", tar.TypeFifo),
				regular("spec.dump", "spec"),
			},
			files: []string{"spec.dump"},
			want:  []string{"spec.dump"},
		},
		{
			name:    "requested symlink",
			entries: []testTarEntry{symlink("config.dump", "../outside/config.dump")},
			files:   []string{"config.dump"},
			wantErr: "config.dump is not a regular file",
		},
		{
			name:    "requested hardlink",
			entries: []testTarEntry{hardlink("spec.dump", "/etc/passwd")},
			files:   []string{"spec.dump"},
			wantErr: "spec.dump is not a regular file",
		},
		{
			name:    "requested device",
			entries: []testTarEntry{special("checkpoint/core-1.img", tar.TypeChar)},
			files:   []string{"checkpoint/core-"},
			wantErr: "checkpoint/core-1.img is not a regular file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			dest := filepath.Join(base, "dest")
			outside := filepath.Join(base, "outside")
			for _, dir := range []string{dest, outside} {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatalf("creating %s: %v", dir, err)
				}
			}

			entries := slices.Clone(tt.entries)
			for i := range entries {
				entries[i].name = strings.ReplaceAll(entries[i].name, absOutside, outside)
			}
			archivePath := filepath.Join(base, "archive.tar")
			writeOrderedTestArchive(t, archivePath, entries)

			untarErr := UntarFiles(archivePath, dest, tt.files)

			// Check for escapes first, as extraction may fail after one.
			// ReadDir also reports directories and symlinks.
			escaped, err := os.ReadDir(outside)
			if err != nil {
				t.Fatalf("reading outside directory: %v", err)
			}
			if len(escaped) != 0 {
				t.Fatalf("entries created outside the destination: %v", escaped)
			}
			if tt.wantErr != "" {
				if untarErr == nil || !strings.Contains(untarErr.Error(), tt.wantErr) {
					t.Fatalf("UntarFiles error = %v, want %q", untarErr, tt.wantErr)
				}
			} else if untarErr != nil {
				t.Fatalf("UntarFiles failed: %v", untarErr)
			}

			if got := listFiles(t, dest); !slices.Equal(got, tt.want) {
				t.Errorf("extracted files = %v, want %v", got, tt.want)
			}
		})
	}
}

// gnuSparseArchive returns a tar archive with a single old GNU sparse file of
// realSize bytes whose only data is stored at offset. Go's tar writer cannot
// create such entries, but GNU tar does by default when run with --sparse.
func gnuSparseArchive(name string, offset int64, data []byte, realSize int64) []byte {
	const blockSize = 512
	header := make([]byte, blockSize)
	octal := func(field []byte, value int64) {
		copy(field, fmt.Sprintf("%0*o", len(field)-1, value))
	}
	copy(header[0:100], name)                // name
	octal(header[100:108], 0o644)            // mode
	octal(header[108:116], 0)                // uid
	octal(header[116:124], 0)                // gid
	octal(header[124:136], int64(len(data))) // size of the stored data
	octal(header[136:148], 0)                // mtime
	header[156] = tar.TypeGNUSparse          // typeflag
	copy(header[257:265], "ustar  \x00")     // magic and version
	octal(header[386:398], offset)           // sp[0].offset
	octal(header[398:410], int64(len(data))) // sp[0].numbytes
	octal(header[483:495], realSize)         // realsize

	// The checksum is computed with the checksum field set to spaces.
	copy(header[148:156], "        ")
	var sum int64
	for _, b := range header {
		sum += int64(b)
	}
	copy(header[148:156], fmt.Sprintf("%06o\x00 ", sum))

	content := make([]byte, (len(data)+blockSize-1)/blockSize*blockSize)
	copy(content, data)
	archive := append(header, content...)
	// An archive ends with two zero blocks.
	return append(archive, make([]byte, 2*blockSize)...)
}

func TestUntarFilesGNUSparse(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "dest")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatalf("creating dest dir: %v", err)
	}

	data := []byte("DATA")
	archivePath := filepath.Join(base, "sparse.tar")
	archive := gnuSparseArchive("checkpoint/pages-1.img", 1024, data, 2048)
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("writing archive: %v", err)
	}

	if err := UntarFiles(archivePath, dest, []string{"checkpoint/pages-"}); err != nil {
		t.Fatalf("UntarFiles failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "checkpoint", "pages-1.img"))
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	want := make([]byte, 2048)
	copy(want[1024:], data)
	if !bytes.Equal(got, want) {
		t.Errorf("extracted %d bytes, want %d bytes with data at offset 1024", len(got), len(want))
	}
}
