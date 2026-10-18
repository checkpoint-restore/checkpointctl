// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"testing"
)

func TestHexdump(t *testing.T) {
	zeros := bytes.Repeat([]byte{0x00}, chunkSize)
	// sameAscii shares the printable-ASCII rendering of zeros ("................")
	// but differs in the first byte.
	sameAscii := append([]byte{0x01}, bytes.Repeat([]byte{0x00}, chunkSize-1)...)
	printable := bytes.Repeat([]byte{'a'}, chunkSize)
	partial := []byte{0x00, 0x00, 0x00, 0x00}

	zerosLine := "0000000000000000  00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  |................|\n"
	sameAsciiLine := "0000000000000010  01 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  |................|\n"
	sameAsciiLineAt20 := "0000000000000020  01 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  |................|\n"
	printableLine := "0000000000000000  61 61 61 61 61 61 61 61 61 61 61 61 61 61 61 61  |aaaaaaaaaaaaaaaa|\n"
	partialLine := "0000000000000010  00 00 00 00  |....|\n"

	tests := []struct {
		name    string
		buf     []byte
		compact bool
		want    string
	}{
		{
			name:    "same ascii but different bytes are not duplicates",
			buf:     append(append([]byte{}, zeros...), sameAscii...),
			compact: true,
			want:    zerosLine + sameAsciiLine,
		},
		{
			name:    "identical rows collapse into one asterisk",
			buf:     bytes.Repeat(zeros, 3),
			compact: true,
			want:    zerosLine + "*\n",
		},
		{
			name:    "duplicate run ends at a row with same ascii",
			buf:     append(append(append([]byte{}, zeros...), zeros...), sameAscii...),
			compact: true,
			want:    zerosLine + "*\n" + sameAsciiLineAt20,
		},
		{
			name:    "non-compact prints every row",
			buf:     bytes.Repeat(zeros, 2),
			compact: false,
			want:    zerosLine + "0000000000000010  00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  |................|\n",
		},
		{
			name:    "partial final row is not a duplicate",
			buf:     append(append([]byte{}, zeros...), partial...),
			compact: true,
			want:    zerosLine + partialLine,
		},
		{
			name:    "printable rows",
			buf:     append(append([]byte{}, printable...), printable...),
			compact: true,
			want:    printableLine + "*\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			hexdump(&out, bytes.NewBuffer(tc.buf), 0, tc.compact)
			if out.String() != tc.want {
				t.Errorf("unexpected output:\ngot:\n%s\nwant:\n%s", out.String(), tc.want)
			}
		})
	}
}
