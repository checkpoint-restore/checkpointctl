// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"math/rand"
	"regexp"
	"testing"
)

func TestParseAddrRange(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantStart uint64
		wantEnd   uint64
		wantErr   bool
	}{
		{
			name:      "valid with 0x prefix",
			input:     "0x1000-0x2000",
			wantStart: 0x1000,
			wantEnd:   0x2000,
			wantErr:   false,
		},
		{
			name:      "valid with 0X uppercase prefix",
			input:     "0X1000-0X2000",
			wantStart: 0x1000,
			wantEnd:   0x2000,
			wantErr:   false,
		},
		{
			name:      "valid without prefix",
			input:     "1000-2000",
			wantStart: 0x1000,
			wantEnd:   0x2000,
			wantErr:   false,
		},
		{
			name:      "valid mixed prefix and surrounding spaces",
			input:     " 0x1000 - 2000 ",
			wantStart: 0x1000,
			wantEnd:   0x2000,
			wantErr:   false,
		},
		{
			name:      "valid 64-bit addresses",
			input:     "0x7f0000000000-0x7f0000010000",
			wantStart: 0x7f0000000000,
			wantEnd:   0x7f0000010000,
			wantErr:   false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "missing end",
			input:   "0x1000-",
			wantErr: true,
		},
		{
			name:    "missing start",
			input:   "-0x2000",
			wantErr: true,
		},
		{
			name:    "no separator",
			input:   "0x1000",
			wantErr: true,
		},
		{
			name:    "multiple separators",
			input:   "0x1000-0x2000-0x3000",
			wantErr: true,
		},
		{
			name:    "invalid hex characters",
			input:   "0x1000-xyz",
			wantErr: true,
		},
		{
			name:    "start equals end",
			input:   "0x1000-0x1000",
			wantErr: true,
		},
		{
			name:    "start greater than end",
			input:   "0x2000-0x1000",
			wantErr: true,
		},
		{
			name:    "overflow",
			input:   "0x1000-0x10000000000000000000",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end, err := parseAddrRange(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseAddrRange(%q) succeeded, expected error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAddrRange(%q) failed: %v", tc.input, err)
			}
			if start != tc.wantStart || end != tc.wantEnd {
				t.Errorf("parseAddrRange(%q) = (%#x, %#x), want (%#x, %#x)",
					tc.input, start, end, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

func fakeMemory(base uint64, data []byte) memReadFunc {
	return func(start, end uint64) ([]byte, error) {
		out := make([]byte, end-start)
		copy(out, data[start-base:end-base])
		return out, nil
	}
}

func TestClipRange(t *testing.T) {
	tests := []struct {
		name               string
		start, end, rs, re uint64
		wantStart, wantEnd uint64
		wantOK             bool
	}{
		{"inside", 0x2000, 0x3000, 0x1000, 0x4000, 0x2000, 0x3000, true},
		{"cut at both edges", 0x1000, 0x5000, 0x2000, 0x3000, 0x2000, 0x3000, true},
		{"cut at start", 0x1000, 0x3000, 0x2000, 0x4000, 0x2000, 0x3000, true},
		{"cut at end", 0x2000, 0x5000, 0x1000, 0x3000, 0x2000, 0x3000, true},
		{"entry below range", 0x1000, 0x2000, 0x2000, 0x3000, 0, 0, false},
		{"entry above range", 0x3000, 0x4000, 0x1000, 0x3000, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, e, ok := clipRange(tt.start, tt.end, tt.rs, tt.re)
			if ok != tt.wantOK || s != tt.wantStart || e != tt.wantEnd {
				t.Errorf("got (%#x, %#x, %v), want (%#x, %#x, %v)", s, e, ok, tt.wantStart, tt.wantEnd, tt.wantOK)
			}
		})
	}
}

func TestSearchClipped(t *testing.T) {
	data := []byte("..PATH=/bin\x00PATH..")
	data[11] = 0
	base := uint64(0x1000)
	end := base + uint64(len(data))

	tests := []struct {
		name      string
		pattern   string
		start     uint64
		end       uint64
		context   int
		chunk     int
		wantAddrs []uint64
		wantMatch []string
	}{
		{"literal", "PATH", base, end, 0, 4, []uint64{base + 2, base + 12}, []string{"PATH", "PATH"}},
		{"literal context clamps at range edge", "PATH", base, end, 5, 4, []uint64{base + 2, base + 12}, []string{"..PATH=/bin", "/bin?PATH.."}},
		{"match crossing the start edge is dropped", "PATH", base + 3, end, 0, 4, []uint64{base + 12}, []string{"PATH"}},
		{"match crossing the end edge is dropped", "PATH", base, base + 14, 0, 4, []uint64{base + 2}, []string{"PATH"}},
		{"match ending on the end edge is kept", "PATH", base, base + 6, 0, 4, []uint64{base + 2}, []string{"PATH"}},
		{"literal match across a chunk boundary", "PATH", base, end, 0, 3, []uint64{base + 2, base + 12}, []string{"PATH", "PATH"}},
		{"regex dot matches sanitized byte", "n.P", base, end, 0, 4, []uint64{base + 10}, []string{"n?P"}},
		{"regex class with question mark", "[ab?]", base, end, 0, 4, []uint64{base + 8, base + 11}, []string{"b", "?"}},
		{"regex without match", "nomatch[0-9]", base, end, 0, 4, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got, err := searchClipped(fakeMemory(base, data), tt.start, tt.end, re, tt.context, tt.chunk)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.wantAddrs) {
				t.Fatalf("got %d matches %+v, want %d", len(got), got, len(tt.wantAddrs))
			}
			for i, m := range got {
				if m.Vaddr != tt.wantAddrs[i] || m.Match != tt.wantMatch[i] {
					t.Errorf("match %d = (%#x, %q), want (%#x, %q)", i, m.Vaddr, m.Match, tt.wantAddrs[i], tt.wantMatch[i])
				}
			}
		})
	}
}

func TestSearchClippedEmptyMatch(t *testing.T) {
	data := []byte("baab")
	re := regexp.MustCompile("a*")
	got, err := searchClipped(fakeMemory(0x1000, data), 0x1000, 0x1004, re, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := re.FindAllIndex(data, -1)
	if len(got) != len(want) {
		t.Fatalf("got %d matches, want %d", len(got), len(want))
	}
	for i, m := range got {
		if m.Vaddr != 0x1000+uint64(want[i][0]) || m.Length != want[i][1]-want[i][0] {
			t.Errorf("match %d = (%#x, %d), want offset %d length %d", i, m.Vaddr, m.Length, want[i][0], want[i][1]-want[i][0])
		}
	}
}

// Verify chunked literal search produces identical results across varying chunk sizes.
func TestSearchClippedLiteralChunks(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	data := make([]byte, 4096)
	for i := range data {
		data[i] = "abc"[rng.Intn(3)]
	}
	base := uint64(0x10000)
	for _, pattern := range []string{"ab", "abca", "aaa", "cbcbc"} {
		re := regexp.MustCompile(pattern)
		want := re.FindAllIndex(data, -1)
		for _, chunk := range []int{1, 2, 3, 7, 64, 5000} {
			got, err := searchClipped(fakeMemory(base, data), base, base+uint64(len(data)), re, 0, chunk)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatalf("pattern %q chunk %d: got %d matches, want %d", pattern, chunk, len(got), len(want))
			}
			for i, m := range got {
				if m.Vaddr != base+uint64(want[i][0]) {
					t.Fatalf("pattern %q chunk %d: match %d at %#x, want %#x", pattern, chunk, i, m.Vaddr, base+uint64(want[i][0]))
				}
			}
		}
	}
}
