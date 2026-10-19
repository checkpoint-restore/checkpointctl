// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestDiffCompressionIndicator(t *testing.T) {
	legacy := newCompressionFixture(t, compressionFixtureOptions{legacy: true, blockPages: 1}).archive(t)
	raw := newCompressionFixture(t, compressionFixtureOptions{blockPages: 1}).archive(t)
	compressed := newCompressionFixture(t, compressionFixtureOptions{compressed: true, blockPages: 2}).archive(t)

	for _, test := range []struct {
		name     string
		archives [2]string
		want     [2]bool
	}{
		{"disabled", [2]string{raw, raw}, [2]bool{false, false}},
		{"enabled", [2]string{compressed, compressed}, [2]bool{true, true}},
		{"enable", [2]string{raw, compressed}, [2]bool{false, true}},
		{"disable", [2]string{compressed, raw}, [2]bool{true, false}},
		{"legacy", [2]string{legacy, compressed}, [2]bool{false, true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := compressionCommand(t, "diff", test.archives[0], test.archives[1], "--format=json")
			if err != nil {
				t.Fatalf("diff: %v: %s", err, output)
			}
			// Pointers distinguish an explicit false from a missing JSON field.
			var result struct {
				A struct {
					Compression *bool `json:"criu_compression"`
				} `json:"checkpoint_a"`
				B struct {
					Compression *bool `json:"criu_compression"`
				} `json:"checkpoint_b"`
			}
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatal(err)
			}
			for i, got := range []*bool{result.A.Compression, result.B.Compression} {
				if got == nil || *got != test.want[i] {
					t.Fatalf("checkpoint %d: expected criu_compression=%t: %s", i, test.want[i], output)
				}
			}

			output, err = compressionCommand(t, "diff", test.archives[0], test.archives[1])
			if err != nil {
				t.Fatalf("diff: %v: %s", err, output)
			}
			for i, label := range []string{"A", "B"} {
				_, section, found := strings.Cut(output, fmt.Sprintf("Checkpoint %s:\n", label))
				section, _, _ = strings.Cut(section, "\n\n")
				state := "Disabled"
				if test.want[i] {
					state = "Enabled"
				}
				indicator := regexp.MustCompile(`(?m)^[\t ]*CRIU compression:[\t ]+` + state + `[\t ]*$`)
				if !found || !indicator.MatchString(section) {
					t.Fatalf("checkpoint %s: expected compression %s: %s", label, state, output)
				}
			}
		})
	}
}

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
