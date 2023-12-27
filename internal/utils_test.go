package internal

import (
	"fmt"
	"testing"
)

func TestFormatTime(t *testing.T) {
	tests := []struct {
		input    uint32
		expected string
	}{
		{1, "1 µs"},
		{500, "500 µs"},
		{999, "999 µs"},
		{1001, "1.001 ms"},
		{1100, "1.1 ms"},
		{13400, "13.4 ms"},
		{1340001, "1.34 s"},
		{1340520, "1.3405 s"},
		{1340560, "1.3406 s"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Input-%d", test.input), func(t *testing.T) {
			result := FormatTime(test.input)
			if result != test.expected {
				t.Errorf("Expected %s, but got %s", test.expected, result)
			}
		})
	}
}
