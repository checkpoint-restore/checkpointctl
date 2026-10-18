// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// These fixtures deliberately use a single printable word for match/context.
func assertSearchRow(t *testing.T, output string, address uint64, match string) {
	t.Helper()

	want := fmt.Sprintf("%016x", address)
	found := 0
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == want && fields[1] == match {
			if instance, err := strconv.Atoi(fields[2]); err != nil || instance < 1 {
				t.Fatalf("invalid match instance: %s", line)
			}
			found++
		}
	}

	if found != 1 {
		t.Fatalf("expected one match at %s with context %q: %s", want, match, output)
	}
}

func verifyMemoryDump(t *testing.T, path string, base uint64, expected []byte) {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	end := base + uint64(len(expected))
	var decoded []byte
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 17 {
			continue
		}
		address, err := strconv.ParseUint(fields[0], 16, 64)
		if err != nil || address < base || address >= end {
			continue
		}
		if address != base+uint64(len(decoded)) {
			t.Fatalf("noncontiguous memory output at %#x", address)
		}
		data, err := hex.DecodeString(strings.Join(fields[1:17], ""))
		if err != nil {
			t.Fatal(err)
		}
		decoded = append(decoded, data...)
	}

	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, expected) {
		t.Fatalf("memory contents differ: decoded %d bytes, expected %d", len(decoded), len(expected))
	}
}
