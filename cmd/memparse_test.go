// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/checkpoint-restore/go-criu/v8/crit"
	core "github.com/checkpoint-restore/go-criu/v8/crit/images/criu-core"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/pagemap"
	"github.com/checkpoint-restore/go-criu/v8/crit/images/pstree"
	"google.golang.org/protobuf/proto"
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

func memparseStateFixture(t *testing.T, rootState, childState uint32) string {
	t.Helper()

	fixture := newCompressionFixture(t, compressionFixtureOptions{blockPages: 1})
	fixture.files["checkpoint/pstree.img"] = compressionImage(t, "PSTREE",
		&pstree.PstreeEntry{Pid: proto.Uint32(1), Ppid: proto.Uint32(0), Threads: []uint32{1}},
		&pstree.PstreeEntry{Pid: proto.Uint32(2), Ppid: proto.Uint32(1), Threads: []uint32{2}},
	)
	for pid, state := range map[string]uint32{"1": rootState, "2": childState} {
		fixture.files["checkpoint/core-"+pid+".img"] = compressionImage(t, "CORE", &core.CoreEntry{
			Tc: &core.TaskCoreEntry{
				TaskState: proto.Uint32(state),
				Comm:      proto.String("demo"),
			},
		})
	}
	fixture.files["checkpoint/mm-2.img"] = fixture.files["checkpoint/mm-1.img"]

	records := []proto.Message{&pagemap.PagemapHead{PagesId: proto.Uint32(1)}}
	for _, entry := range fixture.entries {
		records = append(records, entry)
	}
	fixture.files["checkpoint/pagemap-2.img"] = compressionImage(t, "PAGEMAP", records...)
	return fixture.archive(t)
}

func TestMemparsePidChecksRequestedProcessState(t *testing.T) {
	const (
		alive   = uint32(crit.TaskAlive)
		stopped = uint32(crit.TaskStopped)
		dead    = uint32(crit.TaskDead)
	)

	tests := []struct {
		name       string
		rootState  uint32
		childState uint32
		wantErr    bool
	}{
		// CRIU writes dead tasks only as leaves.
		{name: "dead child under live root", rootState: alive, childState: dead, wantErr: true},
		{name: "stopped child under live root", rootState: alive, childState: stopped},
		// CRIU never writes a dead task with children.
		{name: "live child under dead root", rootState: dead, childState: alive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archive := memparseStateFixture(t, tt.rootState, tt.childState)

			out, err := compressionCommand(t, "memparse", archive, "--pid=2")
			if tt.wantErr {
				if err == nil || !strings.Contains(out, "process 2 has no memory pages") {
					t.Fatalf("expected no memory pages error for process 2: %v: %s", err, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("memparse --pid=2: %v: %s", err, out)
			}
			if !strings.Contains(out, "Displaying memory pages content for process ID 2") {
				t.Fatalf("expected memory pages for process 2: %s", out)
			}
		})
	}
}
