// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v8/crit"
	core "github.com/checkpoint-restore/go-criu/v8/crit/images/criu-core"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/inventory"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/mm"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/pagemap"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/pstree"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/vma"
	"github.com/checkpoint-restore/go-criu/v8/magic"
	"github.com/pierrec/lz4/v4"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Run each command in a separate process: the CLI's flags are package globals.
func TestCompressionCommand(t *testing.T) {
	encoded := os.Getenv("CHECKPOINTCTL_COMPRESSION_COMMAND")
	if encoded == "" {
		return
	}

	var args []string
	if err := json.Unmarshal([]byte(encoded), &args); err != nil {
		t.Fatal(err)
	}

	root := &cobra.Command{
		Use:           "checkpointctl",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(Inspect(), MemParse(), Diff())
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(0)
}

func compressionCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestCompressionCommand$")
	temporary := t.TempDir()
	cmd.Env = append(os.Environ(),
		"CHECKPOINTCTL_COMPRESSION_COMMAND="+string(encoded),
		"TMPDIR="+temporary,
	)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("command %v timed out: %s", args, out)
	}

	entries, readErr := os.ReadDir(temporary)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("command %v leaked temporary files: %v", args, entries)
	}

	return string(out), err
}

// Supply irrelevant required protobuf fields with their defaults, keeping the
// fixture focused on the metadata used by checkpointctl. Optional messages are
// only populated when the fixture explicitly requests them.
func completeRequired(message protoreflect.Message) {
	fields := message.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if field.Cardinality() == protoreflect.Required && !message.Has(field) {
			message.Set(field, message.NewField(field))
		}
		if field.Message() != nil && !field.IsList() && message.Has(field) {
			completeRequired(message.Get(field).Message())
		}
	}
}

func compressionImage(t *testing.T, name string, messages ...proto.Message) []byte {
	t.Helper()
	var out bytes.Buffer
	magics := magic.LoadMagic().ByName
	if name != "INVENTORY" {
		_ = binary.Write(&out, binary.LittleEndian, uint32(magics["IMG_COMMON"]))
	}
	_ = binary.Write(&out, binary.LittleEndian, uint32(magics[name]))

	for _, message := range messages {
		completeRequired(message.ProtoReflect())
		data, err := proto.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		_ = binary.Write(&out, binary.LittleEndian, uint32(len(data)))
		out.Write(data)
	}

	return out.Bytes()
}

type compressionFixture struct {
	files   map[string][]byte
	entries []*pagemap.PagemapEntry
}

type compressionFixtureOptions struct {
	compressed bool
	legacy     bool
	blockPages int
}

const (
	compressionBase = uint64(0x100000)
	compressionArgs = "demo\x00--example\x00"
	compressionEnv  = "CHECKPOINTCTL_TEST=CROSS_BLOCK_NEEDLE\x00EMPTY=\x00"

	// Start before a page or block boundary so decoding must cross it.
	compressionArgBytesBeforeBoundary = 7
	compressionEnvBytesBeforeBoundary = 25
)

// Entries represent adjacent memory mappings. In compressed fixtures, the
// first contains LZ4, raw, and zero-filled blocks. The second stores raw bytes
// without block metadata, and the third contains LZ4 blocks.
// Arguments cross a page boundary; the environment crosses a block boundary.
func newCompressionFixture(t *testing.T, options compressionFixtureOptions) *compressionFixture {
	t.Helper()
	data := make([]byte, 9*pageSize)
	for i := 2 * pageSize; i < 4*pageSize; i++ {
		data[i] = byte(i*17 + i/251)
	}
	for i := 5 * pageSize; i < 8*pageSize; i++ {
		data[i] = byte(i*31 + i/127)
	}
	copy(data[8*pageSize:], bytes.Repeat([]byte("tail"), pageSize/4))

	argOffset := pageSize - compressionArgBytesBeforeBoundary
	envOffset := 2*pageSize - compressionEnvBytesBeforeBoundary
	copy(data[argOffset:], compressionArgs)
	copy(data[envOffset:], compressionEnv)

	fixture := &compressionFixture{files: map[string][]byte{}}
	var payload bytes.Buffer
	spans := [][2]int{{0, 5}, {5, 8}, {8, 9}}
	for _, span := range spans {
		start, end := span[0]*pageSize, span[1]*pageSize
		entry := &pagemap.PagemapEntry{
			Vaddr:         proto.Uint64(compressionBase + uint64(start)),
			CompatNrPages: proto.Uint32(uint32(span[1] - span[0])),
		}
		if !options.legacy {
			entry.NrPages = proto.Uint64(uint64(span[1] - span[0]))
			entry.Flags = proto.Uint32(uint32(crit.PagemapPresent))
		}

		if options.compressed && span[0] == 5 {
			entry.Flags = proto.Uint32(uint32(crit.PagemapPresent | crit.PagemapPayloadAligned))
			padding := (pageSize - payload.Len()%pageSize) % pageSize
			payload.Write(make([]byte, padding))
		}

		if options.compressed && span[0] != 5 {
			entry.Blocks = &pagemap.PagemapBlocks{
				PagesPerBlock:    proto.Uint32(uint32(options.blockPages)),
				TotalPayloadSize: proto.Uint64(0),
			}

			for off := start; off < end; off += options.blockPages * pageSize {
				block := data[off:min(off+options.blockPages*pageSize, end)]
				var stored []byte
				switch {
				case bytes.Equal(block, make([]byte, len(block))):
					// A zero-filled block records its size as zero and stores no bytes.
				case off >= 2*pageSize && off < 4*pageSize:
					stored = block // Explicit raw fallback inside a metadata-bearing entry.
				default:
					encoded := make([]byte, lz4.CompressBlockBound(len(block)))
					n, err := lz4.CompressBlock(block, encoded, nil)
					if err != nil || n == 0 || n >= len(block) {
						t.Fatalf("compress fixture block: n=%d, err=%v", n, err)
					}
					stored = encoded[:n]
				}

				entry.Blocks.BlockSizes = append(entry.Blocks.BlockSizes, uint32(len(stored)))
				*entry.Blocks.TotalPayloadSize += uint64(len(stored))
				payload.Write(stored)
			}
		} else {
			payload.Write(data[start:end])
		}

		fixture.entries = append(fixture.entries, entry)
	}

	fixture.files["checkpoint/pages-1.img"] = payload.Bytes()
	fixture.files["checkpoint/pstree.img"] = compressionImage(t, "PSTREE", &pstree.PstreeEntry{Pid: proto.Uint32(1)})
	fixture.files["checkpoint/core-1.img"] = compressionImage(t, "CORE", &core.CoreEntry{
		Tc: &core.TaskCoreEntry{
			TaskState: proto.Uint32(1),
			Comm:      proto.String("demo"),
		},
	})

	argStart := compressionBase + uint64(argOffset)
	envStart := compressionBase + uint64(envOffset)
	memory := &mm.MmEntry{
		MmArgStart: proto.Uint64(argStart),
		MmArgEnd:   proto.Uint64(argStart + uint64(len(compressionArgs))),
		MmEnvStart: proto.Uint64(envStart),
		MmEnvEnd:   proto.Uint64(envStart + uint64(len(compressionEnv))),
	}
	for _, span := range spans {
		area := &vma.VmaEntry{
			Start: proto.Uint64(compressionBase + uint64(span[0]*pageSize)),
			End:   proto.Uint64(compressionBase + uint64(span[1]*pageSize)),
		}
		completeRequired(area.ProtoReflect())
		memory.Vmas = append(memory.Vmas, area)
	}
	fixture.files["checkpoint/mm-1.img"] = compressionImage(t, "MM", memory)

	if !options.legacy {
		image := &inventory.InventoryEntry{
			ImgVersion: proto.Uint32(2),
			Compress:   proto.Uint32(0),
		}
		if options.compressed {
			image.ImgVersion = proto.Uint32(3)
			image.Compress = proto.Uint32(1)
			image.CompressBlockSize = proto.Uint32(uint32(options.blockPages * pageSize))
		}
		fixture.files["checkpoint/inventory.img"] = compressionImage(t, "INVENTORY", image)
	}

	config, err := json.Marshal(metadata.ContainerConfig{ID: "compression-test", Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.files["config.dump"] = config
	fixture.files["spec.dump"] = []byte(`{"annotations":{"io.container.manager":"libpod"}}`)

	return fixture
}

func (fixture *compressionFixture) archive(t *testing.T) string {
	t.Helper()
	records := []proto.Message{&pagemap.PagemapHead{PagesId: proto.Uint32(1)}}
	for _, entry := range fixture.entries {
		records = append(records, entry)
	}
	fixture.files["checkpoint/pagemap-1.img"] = compressionImage(t, "PAGEMAP", records...)

	path := filepath.Join(t.TempDir(), "checkpoint.tar")
	writeCheckpointArchive(t, path, fixture.files)
	return path
}

func TestInspectCompressionIndicator(t *testing.T) {
	legacy := newCompressionFixture(t, compressionFixtureOptions{legacy: true, blockPages: 1}).archive(t)

	for _, test := range []struct {
		name    string
		options compressionFixtureOptions
		want    bool
	}{
		{name: "legacy", options: compressionFixtureOptions{legacy: true, blockPages: 1}},
		{name: "disabled", options: compressionFixtureOptions{blockPages: 1}},
		{name: "enabled", options: compressionFixtureOptions{compressed: true, blockPages: 2}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCompressionFixture(t, test.options)
			// Default inspection needs only the inventory, not the memory payload.
			delete(fixture.files, "checkpoint/pages-1.img")
			archive := fixture.archive(t)

			output, err := compressionCommand(t, "inspect", archive, legacy, "--format=json")
			if err != nil {
				t.Fatalf("inspect: %v: %s", err, output)
			}
			var nodes []struct {
				Compression *bool `json:"criu_compression"`
			}
			if err := json.Unmarshal([]byte(output), &nodes); err != nil {
				t.Fatal(err)
			}
			if len(nodes) != 2 {
				t.Fatalf("expected two checkpoints: %s", output)
			}
			for i, want := range []bool{test.want, false} {
				if nodes[i].Compression == nil || *nodes[i].Compression != want {
					t.Fatalf("checkpoint %d: expected criu_compression=%t: %s", i, want, output)
				}
			}

			output, err = compressionCommand(t, "inspect", archive)
			want := "CRIU compression: Disabled"
			if test.want {
				want = "CRIU compression: Enabled"
			}
			if err != nil || !strings.Contains(output, want) {
				t.Fatalf("expected %q: %v: %s", want, err, output)
			}
		})
	}
}

func TestInspectCompressionInvalidInventory(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("invalid inventory"),
		compressionImage(t, "INVENTORY", &inventory.InventoryEntry{
			ImgVersion: proto.Uint32(3),
			Compress:   proto.Uint32(99),
		}),
	} {
		fixture := newCompressionFixture(t, compressionFixtureOptions{blockPages: 1})
		fixture.files["checkpoint/inventory.img"] = data
		archive := fixture.archive(t)
		for _, args := range [][]string{
			{"inspect", archive},
			{"inspect", archive, "--format=json"},
		} {
			output, err := compressionCommand(t, args...)
			if err == nil || !strings.Contains(output, "failed to get CRIU compression information") {
				t.Fatalf("expected inventory error for %v: %v: %s", args, err, output)
			}
		}
	}
}

func TestCompressedCheckpointCommands(t *testing.T) {
	raw := newCompressionFixture(t, compressionFixtureOptions{legacy: true, blockPages: 1}).archive(t)

	// Read the uncompressed reference before the subtests so -run cannot
	// skip the baseline when selecting only a compressed case.
	baselinePath := filepath.Join(t.TempDir(), "memory.txt")
	output, err := compressionCommand(t, "memparse", raw, "--pid=1", "--output", baselinePath)
	if err != nil {
		t.Fatalf("write baseline memory: %v\n%s", err, output)
	}
	baselineMemory, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name    string
		options compressionFixtureOptions
	}{
		{name: "legacy", options: compressionFixtureOptions{legacy: true, blockPages: 1}},
		{name: "raw-inventory", options: compressionFixtureOptions{blockPages: 1}},
		{name: "page-blocks", options: compressionFixtureOptions{compressed: true, blockPages: 1}},
		{name: "multi-page-blocks", options: compressionFixtureOptions{compressed: true, blockPages: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			archive := newCompressionFixture(t, test.options).archive(t)

			for _, format := range []string{"tree", "json"} {
				out, err := compressionCommand(t, "inspect", archive, "--ps-tree-cmd", "--ps-tree-env", "--format", format)
				if err != nil {
					t.Fatalf("inspect: %v\n%s", err, out)
				}
				for _, want := range []string{"demo --example", "CROSS_BLOCK_NEEDLE", "EMPTY"} {
					if !strings.Contains(out, want) {
						t.Fatalf("inspect missing %q: %s", want, out)
					}
				}
			}

			out, err := compressionCommand(t, "memparse", archive)
			if err != nil || !strings.Contains(out, metadata.ByteToString(int64(9*pageSize))) {
				t.Fatalf("memory size: %v\n%s", err, out)
			}

			destination := filepath.Join(t.TempDir(), "memory.txt")
			out, err = compressionCommand(t, "memparse", archive, "--pid=1", "--output", destination)
			if err != nil {
				t.Fatalf("write memory: %v\n%s", err, out)
			}
			content, err := os.ReadFile(destination)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(content, baselineMemory) {
				t.Fatal("decoded memory differs from legacy raw checkpoint")
			}

			for _, search := range []string{"--search=CROSS_BLOCK_NEEDLE", "--search-regex=CROSS_BLOCK_N[E]+DLE"} {
				out, err = compressionCommand(t, "memparse", archive, "--pid=1", search, "--context=8")
				if err != nil || !strings.Contains(out, "CROSS_BLOCK_NEEDLE") {
					t.Fatalf("search: %v\n%s", err, out)
				}

				prefix := "CHECKPOINTCTL_TEST="
				envStart := compressionBase + uint64(2*pageSize-compressionEnvBytesBeforeBoundary)
				address := envStart + uint64(len(prefix))
				context := prefix[len(prefix)-8:] + "CROSS_BLOCK_NEEDLE?EMPTY=?"
				assertSearchRow(t, out, address, context)
			}

			out, err = compressionCommand(t, "diff", raw, archive, "--ps-tree-cmd", "--ps-tree-env", "--format=json")
			if err != nil {
				t.Fatalf("diff: %v\n%s", err, out)
			}
			var result DiffResult
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			changes := result.ProcessChanges
			if changes == nil ||
				len(changes.Unchanged) != 1 ||
				len(changes.Modified) != 0 ||
				len(changes.Added) != 0 ||
				len(changes.Removed) != 0 {
				t.Fatalf("unexpected process changes: %+v", changes)
			}
		})
	}
}

func TestCompressedCheckpointErrors(t *testing.T) {
	tests := []struct {
		name   string
		want   string
		mutate func(*compressionFixture)
	}{
		{
			name: "missing inventory",
			want: "requires inventory.img",
			mutate: func(f *compressionFixture) {
				delete(f.files, "checkpoint/inventory.img")
			},
		},
		{
			name: "invalid inventory",
			want: "invalid compression mode",
			mutate: func(f *compressionFixture) {
				f.files["checkpoint/inventory.img"] = compressionImage(t, "INVENTORY", &inventory.InventoryEntry{
					ImgVersion: proto.Uint32(3),
					Compress:   proto.Uint32(99),
				})
			},
		},
		{
			name: "wrong image version",
			want: "cannot contain compressed",
			mutate: func(f *compressionFixture) {
				f.files["checkpoint/inventory.img"] = compressionImage(t, "INVENTORY", &inventory.InventoryEntry{
					ImgVersion: proto.Uint32(2),
					Compress:   proto.Uint32(1),
				})
			},
		},
		{
			name: "block granularity exceeds limit",
			want: "block size exceeds maximum",
			mutate: func(f *compressionFixture) {
				f.entries[0].Blocks.PagesPerBlock = proto.Uint32(uint32(4*1024*1024/pageSize + 1))
			},
		},
		{
			name: "zero block granularity",
			want: "pages_per_block is zero",
			mutate: func(f *compressionFixture) {
				f.entries[0].Blocks.PagesPerBlock = proto.Uint32(0)
			},
		},
		{
			name: "aligned nonpresent",
			want: "non-present entry",
			mutate: func(f *compressionFixture) {
				f.entries[0].Blocks = nil
				f.entries[0].Flags = proto.Uint32(uint32(crit.PagemapPayloadAligned))
			},
		},
		{
			name: "block count",
			want: "compressed blocks, expected",
			mutate: func(f *compressionFixture) {
				f.entries[0].Blocks.BlockSizes = f.entries[0].Blocks.BlockSizes[:1]
			},
		},
		{
			name: "payload total",
			want: "does not match block sum",
			mutate: func(f *compressionFixture) {
				*f.entries[0].Blocks.TotalPayloadSize++
			},
		},
		{
			name: "oversized block",
			want: "exceeds its uncompressed size",
			mutate: func(f *compressionFixture) {
				f.entries[0].Blocks.BlockSizes[0] = uint32(2*pageSize + 1)
			},
		},
		{
			name: "short final block",
			want: "exceeds its uncompressed size",
			mutate: func(f *compressionFixture) {
				f.entries[0].Blocks.BlockSizes[2] = uint32(pageSize + 1)
			},
		},
		{
			name: "truncated payload",
			want: "unexpected EOF",
			mutate: func(f *compressionFixture) {
				f.files["checkpoint/pages-1.img"] = []byte{1}
			},
		},
		{
			name: "corrupt LZ4",
			want: "lz4:",
			mutate: func(f *compressionFixture) {
				clear(f.files["checkpoint/pages-1.img"][:f.entries[0].Blocks.BlockSizes[0]])
			},
		},
		{
			name: "missing parent",
			want: "open parent for",
			mutate: func(f *compressionFixture) {
				f.entries[0].Blocks = nil
				f.entries[0].Flags = proto.Uint32(uint32(crit.PagemapParent))
			},
		},
		{
			name: "unavailable lazy page",
			want: "lazy page",
			mutate: func(f *compressionFixture) {
				f.entries[0].Blocks = nil
				f.entries[0].Flags = proto.Uint32(uint32(crit.PagemapLazy))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCompressionFixture(t, compressionFixtureOptions{compressed: true, blockPages: 2})
			test.mutate(fixture)

			archive := fixture.archive(t)
			out, err := compressionCommand(t, "inspect", archive, "--ps-tree-cmd")
			if err == nil || !strings.Contains(out, test.want) {
				t.Fatalf("got err=%v, output=%s; want %q", err, out, test.want)
			}
		})
	}
}

func TestMemorySizeOverflow(t *testing.T) {
	fixture := newCompressionFixture(t, compressionFixtureOptions{blockPages: 1})
	fixture.entries = []*pagemap.PagemapEntry{{
		Vaddr:         proto.Uint64(0),
		CompatNrPages: proto.Uint32(0),
		NrPages:       proto.Uint64(uint64(1) << 63 / uint64(pageSize)),
		Flags:         proto.Uint32(uint32(crit.PagemapLazy)),
	}}

	out, err := compressionCommand(t, "memparse", fixture.archive(t))
	if err == nil || !strings.Contains(out, "exceeds supported size") {
		t.Fatalf("expected size overflow, got %v: %s", err, out)
	}
}
