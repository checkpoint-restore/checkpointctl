// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/checkpoint-restore/go-criu/v8/crit"
)

// parseAddrRange parses a hexadecimal "start-end" virtual address range.
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

// rangeSearchChunk represents the chunk size (in bytes) to read at a time, matching go-criu.
const rangeSearchChunk = 10 * 1024 * 1024

type memReadFunc func(start, end uint64) ([]byte, error)

// clipRange intersects [start, end) with [rangeStart, rangeEnd).
func clipRange(start, end, rangeStart, rangeEnd uint64) (uint64, uint64, bool) {
	clipStart := max(start, rangeStart)
	clipEnd := min(end, rangeEnd)
	if clipStart >= clipEnd {
		return 0, 0, false
	}
	return clipStart, clipEnd, true
}

// sanitizeBytes replaces non-printable bytes with '?' to match go-criu pattern matching.
func sanitizeBytes(data []byte) {
	for i, b := range data {
		if b < 32 || b >= 127 {
			data[i] = '?'
		}
	}
}

// searchClipped searches [start, end) for matches on sanitized memory.
func searchClipped(read memReadFunc, start, end uint64, re *regexp.Regexp, contextSize, chunk int) ([]crit.PatternMatch, error) {
	if literal, full := re.LiteralPrefix(); full && len(literal) > 0 {
		return searchClippedLiteral(read, start, end, []byte(literal), contextSize, chunk)
	}

	data, err := read(start, end)
	if err != nil {
		return nil, err
	}
	sanitizeBytes(data)

	var results []crit.PatternMatch
	for _, idx := range re.FindAllIndex(data, -1) {
		cStart := max(idx[0]-contextSize, 0)
		cEnd := min(idx[1]+contextSize, len(data))
		results = append(results, crit.PatternMatch{
			Vaddr:   start + uint64(idx[0]),
			Length:  idx[1] - idx[0],
			Context: contextSize,
			Match:   string(data[cStart:cEnd]),
		})
	}
	return results, nil
}

// searchClippedLiteral searches [start, end) for a literal pattern in chunks.
func searchClippedLiteral(read memReadFunc, start, end uint64, literal []byte, contextSize, chunk int) ([]crit.PatternMatch, error) {
	var results []crit.PatternMatch
	litLen := uint64(len(literal))

	pos := start
	for pos < end {
		// Overlap by pattern length to capture matches straddling chunk boundaries.
		windowEnd := min(pos+uint64(chunk)+litLen-1, end)
		if windowEnd < pos+litLen {
			break
		}
		window, err := read(pos, windowEnd)
		if err != nil {
			return nil, err
		}
		sanitizeBytes(window)

		i := bytes.Index(window, literal)
		if i < 0 {
			if windowEnd == end {
				break
			}
			pos += uint64(chunk)
			continue
		}
		matchStart := pos + uint64(i)
		matchEnd := matchStart + litLen

		cStart := max(matchStart-min(matchStart-start, uint64(contextSize)), start)
		cEnd := min(matchEnd+uint64(contextSize), end)
		ctx, err := read(cStart, cEnd)
		if err != nil {
			return nil, err
		}
		sanitizeBytes(ctx)

		results = append(results, crit.PatternMatch{
			Vaddr:   matchStart,
			Length:  len(literal),
			Context: contextSize,
			Match:   string(ctx),
		})
		pos = matchEnd
	}
	return results, nil
}
