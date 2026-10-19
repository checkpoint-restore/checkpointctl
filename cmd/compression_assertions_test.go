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

// Expand '*' rows in compact memory output within the expected address range.
func verifyCompactMapping(t *testing.T, output string, base uint64, expected []byte) {
	t.Helper()

	decoded, err := compactMapping(output, base, len(expected))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, expected) {
		t.Fatalf("compact memory differs: got %d bytes, expected %d", len(decoded), len(expected))
	}
}

// Known mappings end with a distinct row, so their full length must be
// visible in the output. Never infer the end from a trailing '*'.
func compactMapping(output string, base uint64, size int) ([]byte, error) {
	end := base + uint64(size)
	var decoded []byte

	add := func(address uint64, data []byte) error {
		if address < base || address >= end {
			return nil
		}
		if address != base+uint64(len(decoded)) {
			return fmt.Errorf("compact output has a gap at %#x", address)
		}
		decoded = append(decoded, data...)
		return nil
	}

	var previous []byte
	var previousAddress, lastRow uint64
	repeating := false
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "*" {
			repeating = true
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 17 {
			continue
		}
		address, err := strconv.ParseUint(fields[0], 16, 64)
		if err != nil {
			continue
		}
		data, err := hex.DecodeString(strings.Join(fields[1:17], ""))
		if err != nil {
			return nil, err
		}

		if repeating {
			repeatStart := max(base, previousAddress+16)
			repeatEnd := min(end, address)
			for row := repeatStart; row < repeatEnd; row += 16 {
				if err := add(row, previous); err != nil {
					return nil, err
				}
			}
		}
		if err := add(address, data); err != nil {
			return nil, err
		}
		if address >= base && address < end {
			lastRow = address
		}

		previous = data
		previousAddress = address
		repeating = false
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if lastRow+16 != end {
		return nil, fmt.Errorf("compact output is missing the final row at %#x", end-16)
	}
	return decoded, nil
}

func TestCompactMapping(t *testing.T) {
	const base = uint64(0x1000)
	row := bytes.Repeat([]byte{'K'}, 16)
	final := []byte("END_OF_MAPPING!!")
	firstLine := fmt.Sprintf("%016x  % x |KKKKKKKKKKKKKKKK|\n", base, row)
	lastLine := fmt.Sprintf("%016x  % x |END_OF_MAPPING!!|\n", base+48, final)

	for _, test := range []struct {
		name, output string
		wantError    bool
	}{
		{"complete", firstLine + "*\n" + lastLine, false},
		{"truncated", firstLine + "*\n", true},
		{"gap", firstLine + lastLine, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			actual, err := compactMapping(test.output, base, 64)
			if test.wantError {
				if err == nil {
					t.Fatal("incomplete output was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			expected := append(bytes.Repeat(row, 3), final...)
			if !bytes.Equal(actual, expected) {
				t.Fatalf("decoded %x, want %x", actual, expected)
			}
		})
	}
}

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
