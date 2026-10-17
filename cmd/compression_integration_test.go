// SPDX-License-Identifier: Apache-2.0
//go:build linux

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/checkpoint-restore/go-criu/v8/crit"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/pagemap"
)

// Match the C workload's VMA boundaries. Generate expected bytes separately
// from checkpointctl so reader errors cannot affect the expected contents.
const (
	workloadFirstVMAEnd     = 13
	workloadSecondVMAEnd    = 47
	workloadCompressibleEnd = 1153
	workloadRandomEnd       = 1282
	workloadPages           = 1536
	workloadSentinelStart   = workloadPages - 1
)

// Explicitly requested tests must fail, never skip missing features.
// Run with root privileges and CRIU_TEST_BINARY pointing to an LZ4-enabled CRIU.
func TestCRIUCompressedCheckpoints(t *testing.T) {
	h := newCompressionHarness(t)
	process, readyData := h.workload(t, "workload", nil, nil)
	workload := filepath.Join(h.directory, "workload")

	var address uint64
	var page int
	if _, err := fmt.Sscanf(string(readyData), "%d %d", &address, &page); err != nil {
		t.Fatalf("workload readiness: %q: %v", readyData, err)
	}

	expected := make([]byte, workloadPages*page)
	random := uint32(0x12345678)
	for i := range expected {
		switch {
		case i < workloadCompressibleEnd*page:
			expected[i] = 'Z'
		case i < workloadRandomEnd*page:
			random ^= random << 13
			random ^= random >> 17
			random ^= random << 5
			expected[i] = byte(random)
		}
	}
	copy(expected[len(expected)-16:], "END_OF_MAPPING!!")

	// Always create a raw baseline, including when -run selects one mode.
	rawImages, rawArchive := h.dump(t, process.Process.Pid, "raw", 0)
	qualifyCompressionLayout(t, rawImages, process.Process.Pid, page, 0, address)
	baselineSize := memorySizeTable(t, h.run(t, "memparse", rawArchive))

	for _, mode := range []struct {
		name  string
		block int
	}{
		{"raw", 0},
		{"page", page},
		{"256K", 256 * 1024},
		{"1M", 1024 * 1024},
		{"4M", 4 * 1024 * 1024},
	} {
		t.Run(mode.name, func(t *testing.T) {
			archive := rawArchive
			if mode.block != 0 {
				images, compressedArchive := h.dump(t, process.Process.Pid, mode.name, mode.block)
				qualifyCompressionLayout(t, images, process.Process.Pid, page, mode.block, address)
				archive = compressedArchive
			}

			for _, format := range []string{"tree", "json"} {
				output := h.run(t, "inspect", archive, "--ps-tree-cmd", "--ps-tree-env", "--format", format)
				for _, want := range []string{"CRIU_COMPRESSION_MARKER", workload, "CHECKPOINTCTL_EMPTY"} {
					if !strings.Contains(output, want) {
						t.Fatalf("inspect missing %q: %s", want, output)
					}
				}
			}
			output := h.run(t, "inspect", archive, "--all", "--format=json")
			if !strings.Contains(output, "CRIU_COMPRESSION_MARKER") || !strings.Contains(output, workload) {
				t.Fatalf("inspect --all did not decode process memory: %s", output)
			}

			table := memorySizeTable(t, h.run(t, "memparse", archive))
			if table != baselineSize {
				t.Fatalf("logical memory size changed:\n%s\n%s", baselineSize, table)
			}

			for _, search := range []string{
				"--search=CRIU_COMPRESSION_MARKER",
				"--search-regex=CRIU_COMPRESSION_M[A]+RKER",
			} {
				output := h.run(t, "memparse", archive, "--pid", strconv.Itoa(process.Process.Pid), search, "--context=8")
				if !strings.Contains(output, "CRIU_COMPRESSION_MARKER") {
					t.Fatalf("search failed: %s", output)
				}
			}

			outputFile := filepath.Join(h.directory, "memory-"+mode.name)
			if err := os.WriteFile(outputFile, nil, 0o666); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(outputFile, 0o666); err != nil {
				t.Fatal(err)
			}
			h.run(t, "memparse", archive, "--pid", strconv.Itoa(process.Process.Pid), "--output", outputFile)
			verifyMemoryDump(t, outputFile, address, expected)
			compact := h.run(t, "memparse", archive, "--pid", strconv.Itoa(process.Process.Pid))
			verifyCompactMapping(t, compact, address, expected)

			output = h.run(t, "diff", rawArchive, archive, "--ps-tree-cmd", "--ps-tree-env", "--format=json")
			var result DiffResult
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatal(err)
			}
			if mode.block != 0 && (result.MemoryChanges == nil || result.MemoryChanges.SizeChangeBytes >= 0) {
				t.Fatalf("compressed page images should be smaller than the raw baseline: %+v", result.MemoryChanges)
			}

			changes := result.ProcessChanges
			if changes == nil {
				t.Fatal("missing process changes")
			}
			if len(changes.Unchanged) != 1 || len(changes.Modified) != 0 {
				t.Fatalf("process metadata changed: %+v", changes)
			}
		})
	}
}

func memorySizeTable(t *testing.T, output string) string {
	t.Helper()

	// Ignore the introductory line containing the archive path.
	_, table, found := strings.Cut(output, "PID")
	if !found {
		t.Fatalf("missing memory table: %s", output)
	}
	return table
}

func qualifyCompressionLayout(t *testing.T, directory string, pid, page, block int, base uint64) {
	t.Helper()

	file, err := os.Open(filepath.Join(directory, fmt.Sprintf("pagemap-%d.img", pid)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image, err := crit.New(file, nil, "", false, false).Decode(&pagemap.PagemapHead{})
	if err != nil {
		t.Fatal(err)
	}
	if len(image.Entries) == 0 {
		t.Fatal("pagemap image has no header")
	}

	info, err := crit.ReadCompressionInfo(directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Compressed() != (block != 0) || (block != 0 && int(info.BlockSizeBytes) != block) {
		t.Fatalf("unexpected compression inventory: %+v", info)
	}

	boundaries := []uint64{
		workloadFirstVMAEnd,
		workloadSecondVMAEnd,
		workloadCompressibleEnd,
		workloadRandomEnd,
		workloadSentinelStart,
	}
	mappingEnd := base + uint64(workloadPages*page)
	var lz4, zero, raw, aligned, short bool
	for _, record := range image.Entries[1:] {
		entry := record.Message.(*pagemap.PagemapEntry)
		pages := crit.PagemapPageCount(entry)
		start := entry.GetVaddr()
		end := start + pages*uint64(page)

		for _, boundary := range boundaries {
			vaddr := base + boundary*uint64(page)
			if block != 0 && start < vaddr && end > vaddr {
				t.Fatalf("entry crosses workload VMA boundary %#x", vaddr)
			}
		}
		if start < base || end > mappingEnd {
			continue
		}

		if entry.Blocks == nil && crit.EffectivePagemapFlags(entry)&crit.PagemapPayloadAligned != 0 {
			aligned = true
		}
		if entry.Blocks == nil {
			raw = true
			continue
		}

		pagesPerBlock := uint64(entry.Blocks.GetPagesPerBlock())
		if int(pagesPerBlock) != block/page {
			t.Fatalf("unexpected pages_per_block: %d", entry.Blocks.GetPagesPerBlock())
		}
		remaining := pages
		for _, stored := range entry.Blocks.BlockSizes {
			pages := min(remaining, pagesPerBlock)
			decoded := pages * uint64(page)
			switch {
			case stored == 0:
				zero = true
			case uint64(stored) == decoded:
				raw = true
			default:
				lz4 = true
			}
			if pages < pagesPerBlock {
				short = true
			}
			remaining -= pages
		}
	}

	if block != 0 && (!lz4 || !zero || !raw || !aligned || (block > page && !short)) {
		t.Fatalf("fixture lacks required representations: lz4=%t zero=%t raw=%t aligned=%t short=%t", lz4, zero, raw, aligned, short)
	}
}
