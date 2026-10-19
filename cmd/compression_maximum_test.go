// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/checkpoint-restore/go-criu/v8/crit"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/inventory"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/mm"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/pagemap"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/vma"
	"github.com/pierrec/lz4/v4"
	"google.golang.org/protobuf/proto"
)

// CRIU can split entries below the requested block size because
// of pipe capacity. Independently exercise a full 4 MiB encoded block here.
func TestCompressedMaximumBlock(t *testing.T) {
	const blockSize = 4 * 1024 * 1024
	pagesPerBlock := uint32(blockSize / pageSize)
	raw := newCompressionFixture(t, compressionFixtureOptions{blockPages: 1})

	data := bytes.Repeat([]byte{'Z'}, blockSize)
	copy(data, raw.files["checkpoint/pages-1.img"])
	raw.files["checkpoint/pages-1.img"] = data

	raw.entries = []*pagemap.PagemapEntry{{
		Vaddr:         proto.Uint64(compressionBase),
		CompatNrPages: proto.Uint32(pagesPerBlock),
		NrPages:       proto.Uint64(uint64(pagesPerBlock)),
		Flags:         proto.Uint32(uint32(crit.PagemapPresent)),
	}}

	area := &vma.VmaEntry{
		Start: proto.Uint64(compressionBase),
		End:   proto.Uint64(compressionBase + blockSize),
	}
	completeRequired(area.ProtoReflect())

	argOffset := pageSize - compressionArgBytesBeforeBoundary
	envOffset := 2*pageSize - compressionEnvBytesBeforeBoundary
	argStart := compressionBase + uint64(argOffset)
	envStart := compressionBase + uint64(envOffset)

	memory := &mm.MmEntry{
		MmArgStart: proto.Uint64(argStart),
		MmArgEnd:   proto.Uint64(argStart + uint64(len(compressionArgs))),
		MmEnvStart: proto.Uint64(envStart),
		MmEnvEnd:   proto.Uint64(envStart + uint64(len(compressionEnv))),
		Vmas:       []*vma.VmaEntry{area},
	}
	raw.files["checkpoint/mm-1.img"] = compressionImage(t, "MM", memory)

	encoded := make([]byte, lz4.CompressBlockBound(blockSize))
	n, err := lz4.CompressBlock(data, encoded, nil)
	if err != nil || n <= 0 || n >= blockSize {
		t.Fatalf("encode maximum block: n=%d, err=%v", n, err)
	}

	for _, kind := range []string{"raw", "lz4", "packed-raw"} {
		entry := proto.Clone(raw.entries[0]).(*pagemap.PagemapEntry)
		fixture := &compressionFixture{
			files:   map[string][]byte{},
			entries: []*pagemap.PagemapEntry{entry},
		}
		for name, value := range raw.files {
			fixture.files[name] = value
		}

		if kind != "raw" {
			stored := encoded[:n]
			if kind == "packed-raw" {
				stored = data
			}

			fixture.entries[0].Blocks = &pagemap.PagemapBlocks{
				PagesPerBlock:    proto.Uint32(pagesPerBlock),
				BlockSizes:       []uint32{uint32(len(stored))},
				TotalPayloadSize: proto.Uint64(uint64(len(stored))),
			}
			fixture.files["checkpoint/pages-1.img"] = stored

			image := &inventory.InventoryEntry{
				ImgVersion:        proto.Uint32(3),
				Compress:          proto.Uint32(1),
				CompressBlockSize: proto.Uint32(blockSize),
			}
			fixture.files["checkpoint/inventory.img"] = compressionImage(t, "INVENTORY", image)
		}

		archive := fixture.archive(t)
		output, err := compressionCommand(t, "inspect", archive, "--ps-tree-cmd", "--ps-tree-env", "--format=json")
		if err != nil {
			t.Fatalf("maximum-block inspection: %v: %s", err, output)
		}

		destination := filepath.Join(t.TempDir(), "memory")
		output, err = compressionCommand(t, "memparse", archive, "--pid=1", "--output", destination)
		if err != nil {
			t.Fatalf("write maximum-block memory: %v: %s", err, output)
		}

		verifyMemoryDump(t, destination, compressionBase, data)
	}
}
