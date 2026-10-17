// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
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
