// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiffCompressedMemory(t *testing.T) {
	rawFixture := newCompressionFixture(t, compressionFixtureOptions{legacy: true, blockPages: 1})
	compressedFixture := newCompressionFixture(t, compressionFixtureOptions{compressed: true, blockPages: 2})
	rawSize := len(rawFixture.files["checkpoint/pages-1.img"])
	compressedSize := len(compressedFixture.files["checkpoint/pages-1.img"])
	storedChange := int64(compressedSize - rawSize)

	raw := rawFixture.archive(t)
	compressed := compressedFixture.archive(t)
	output, err := compressionCommand(t, "diff", raw, compressed, "--ps-tree-cmd", "--ps-tree-env", "--format=json")
	if err != nil {
		t.Fatalf("diff: %v: %s", err, output)
	}

	var result DiffResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result.MemoryChanges == nil || result.MemoryChanges.SizeChangeBytes != storedChange {
		t.Fatalf("incorrect stored page-image size change: %+v", result.MemoryChanges)
	}

	delete(compressedFixture.files, "checkpoint/inventory.img")
	invalid := compressedFixture.archive(t)
	for _, archives := range [][2]string{{raw, invalid}, {invalid, raw}} {
		output, err := compressionCommand(t, "diff", archives[0], archives[1], "--ps-tree-cmd", "--format=json")
		if err == nil || !strings.Contains(output, "requires inventory.img") {
			t.Fatalf("expected missing inventory error: %v: %s", err, output)
		}
	}
}
