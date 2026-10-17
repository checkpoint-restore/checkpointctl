// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"strconv"
	"strings"
)

// parseAddrRange parses a "start-end" virtual address range. Both addresses
// accept hexadecimal with or without the 0x prefix. It rejects unparsable
// input, empty strings, and ranges where start >= end.
func parseAddrRange(s string) (uint64, uint64, error) {
	if s == "" {
		return 0, 0, fmt.Errorf("invalid address range: cannot be empty")
	}
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid address range %q: expected format \"start-end\"", s)
	}
	start, err := parseAddr(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid address range %q: %w", s, err)
	}
	end, err := parseAddr(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid address range %q: %w", s, err)
	}
	if start >= end {
		return 0, 0, fmt.Errorf("invalid address range %q: start must be smaller than end", s)
	}
	return start, end, nil
}

// parseAddr parses a single hexadecimal virtual address.
func parseAddr(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return 0, fmt.Errorf("empty address")
	}
	addr, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid address %q: %w", s, err)
	}
	return addr, nil
}
