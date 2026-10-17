// SPDX-License-Identifier: Apache-2.0

package cmd

import (
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
