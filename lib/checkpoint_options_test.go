// SPDX-License-Identifier: Apache-2.0

package metadata

import (
	"strings"
	"testing"
)

func TestParseBool(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantBool  bool
		wantError bool
	}{
		// True values
		{name: "yes lowercase", value: "yes", wantBool: true, wantError: false},
		{name: "Yes capitalized", value: "Yes", wantBool: true, wantError: false},
		{name: "YES uppercase", value: "YES", wantBool: true, wantError: false},
		{name: "true lowercase", value: "true", wantBool: true, wantError: false},
		{name: "True capitalized", value: "True", wantBool: true, wantError: false},
		{name: "TRUE uppercase", value: "TRUE", wantBool: true, wantError: false},
		{name: "on lowercase", value: "on", wantBool: true, wantError: false},
		{name: "On capitalized", value: "On", wantBool: true, wantError: false},
		{name: "ON uppercase", value: "ON", wantBool: true, wantError: false},
		{name: "1", value: "1", wantBool: true, wantError: false},
		// False values
		{name: "no lowercase", value: "no", wantBool: false, wantError: false},
		{name: "No capitalized", value: "No", wantBool: false, wantError: false},
		{name: "NO uppercase", value: "NO", wantBool: false, wantError: false},
		{name: "false lowercase", value: "false", wantBool: false, wantError: false},
		{name: "False capitalized", value: "False", wantBool: false, wantError: false},
		{name: "FALSE uppercase", value: "FALSE", wantBool: false, wantError: false},
		{name: "off lowercase", value: "off", wantBool: false, wantError: false},
		{name: "Off capitalized", value: "Off", wantBool: false, wantError: false},
		{name: "OFF uppercase", value: "OFF", wantBool: false, wantError: false},
		{name: "0", value: "0", wantBool: false, wantError: false},
		// Invalid values
		{name: "invalid string", value: "invalid", wantBool: false, wantError: true},
		{name: "empty string", value: "", wantBool: false, wantError: true},
		{name: "yEs mixed case", value: "yEs", wantBool: false, wantError: true},
		{name: "number 2", value: "2", wantBool: false, wantError: true},
		{name: "whitespace", value: " yes", wantBool: false, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBool(tt.value)
			if tt.wantError {
				if err == nil {
					t.Errorf("parseBool(%q) expected error, got nil", tt.value)
				}
				return
			}
			if err != nil {
				t.Errorf("parseBool(%q) unexpected error: %v", tt.value, err)
				return
			}
			if got != tt.wantBool {
				t.Errorf("parseBool(%q) = %v, want %v", tt.value, got, tt.wantBool)
			}
		})
	}
}

func TestParseCheckpointOptions(t *testing.T) {
	tests := []struct {
		name           string
		options        map[string]string
		wantLeave      bool
		wantTCP        bool
		wantGhost      int64
		wantNetwork    string
		wantError      bool
		wantErrStrings []string
	}{
		{
			name:    "empty options",
			options: map[string]string{},
		},
		{
			name:    "nil options",
			options: nil,
		},
		{
			name: "leave-running true",
			options: map[string]string{
				"leave-running": "yes",
			},
			wantLeave: true,
		},
		{
			name: "leave-running false",
			options: map[string]string{
				"leave-running": "no",
			},
			wantLeave: false,
		},
		{
			name: "tcp-established true",
			options: map[string]string{
				"tcp-established": "true",
			},
			wantTCP: true,
		},
		{
			name: "tcp-established false",
			options: map[string]string{
				"tcp-established": "0",
			},
			wantTCP: false,
		},
		{
			name: "ghost-limit valid",
			options: map[string]string{
				"ghost-limit": "1048576",
			},
			wantGhost: 1048576,
		},
		{
			name: "ghost-limit zero",
			options: map[string]string{
				"ghost-limit": "0",
			},
			wantGhost: 0,
		},
		{
			name: "ghost-limit negative",
			options: map[string]string{
				"ghost-limit": "-100",
			},
			wantGhost: -100,
		},
		{
			name: "network-lock valid",
			options: map[string]string{
				"network-lock": "nftables",
			},
			wantNetwork: "nftables",
		},
		{
			name: "network-lock empty string",
			options: map[string]string{
				"network-lock": "",
			},
			wantNetwork: "",
		},
		{
			name: "all options combined",
			options: map[string]string{
				"leave-running":   "on",
				"tcp-established": "1",
				"ghost-limit":     "2097152",
				"network-lock":    "iptables",
			},
			wantLeave:   true,
			wantTCP:     true,
			wantGhost:   2097152,
			wantNetwork: "iptables",
		},
		{
			name: "unknown option",
			options: map[string]string{
				"unknown-option": "value",
			},
			wantError:      true,
			wantErrStrings: []string{"unknown option", "unknown-option"},
		},
		{
			name: "invalid boolean value",
			options: map[string]string{
				"leave-running": "maybe",
			},
			wantError:      true,
			wantErrStrings: []string{"leave-running", "invalid boolean value"},
		},
		{
			name: "invalid integer value",
			options: map[string]string{
				"ghost-limit": "not-a-number",
			},
			wantError:      true,
			wantErrStrings: []string{"ghost-limit", "invalid integer value"},
		},
		{
			name: "integer with text",
			options: map[string]string{
				"ghost-limit": "100abc",
			},
			wantGhost: 100,
		},
		{
			name: "multiple errors",
			options: map[string]string{
				"unknown-option":  "value",
				"leave-running":   "invalid",
				"tcp-established": "also-invalid",
			},
			wantError:      true,
			wantErrStrings: []string{"unknown option", "leave-running", "tcp-established"},
		},
		{
			name: "valid and invalid options mixed",
			options: map[string]string{
				"leave-running": "yes",
				"ghost-limit":   "not-valid",
			},
			wantLeave:      true,
			wantError:      true,
			wantErrStrings: []string{"ghost-limit", "invalid integer value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCheckpointOptions(tt.options)

			if tt.wantError {
				if err == nil {
					t.Errorf("ParseCheckpointOptions() expected error, got nil")
					return
				}
				for _, substr := range tt.wantErrStrings {
					if !strings.Contains(err.Error(), substr) {
						t.Errorf("error %q should contain %q", err.Error(), substr)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ParseCheckpointOptions() unexpected error: %v", err)
					return
				}
			}

			if result == nil {
				t.Errorf("ParseCheckpointOptions() returned nil result")
				return
			}

			if result.LeaveRunning != tt.wantLeave {
				t.Errorf("LeaveRunning = %v, want %v", result.LeaveRunning, tt.wantLeave)
			}
			if result.TCPEstablished != tt.wantTCP {
				t.Errorf("TCPEstablished = %v, want %v", result.TCPEstablished, tt.wantTCP)
			}
			if result.GhostLimit != tt.wantGhost {
				t.Errorf("GhostLimit = %v, want %v", result.GhostLimit, tt.wantGhost)
			}
			if result.NetworkLock != tt.wantNetwork {
				t.Errorf("NetworkLock = %q, want %q", result.NetworkLock, tt.wantNetwork)
			}
		})
	}
}

func TestOptionTypeConstants(t *testing.T) {
	// Verify the option types are distinct
	if OptionTypeBool == OptionTypeInt {
		t.Error("OptionTypeBool should not equal OptionTypeInt")
	}
	if OptionTypeBool == OptionTypeString {
		t.Error("OptionTypeBool should not equal OptionTypeString")
	}
	if OptionTypeInt == OptionTypeString {
		t.Error("OptionTypeInt should not equal OptionTypeString")
	}
}

func TestKnownCheckpointOptions(t *testing.T) {
	// Verify known options are properly defined
	expectedOptions := map[string]OptionType{
		"leave-running":   OptionTypeBool,
		"tcp-established": OptionTypeBool,
		"ghost-limit":     OptionTypeInt,
		"network-lock":    OptionTypeString,
	}

	for name, expectedType := range expectedOptions {
		opt, exists := knownCheckpointOptions[name]
		if !exists {
			t.Errorf("expected option %q to exist in knownCheckpointOptions", name)
			continue
		}
		if opt.Type != expectedType {
			t.Errorf("option %q has type %v, want %v", name, opt.Type, expectedType)
		}
	}

	if len(knownCheckpointOptions) != len(expectedOptions) {
		t.Errorf("knownCheckpointOptions has %d entries, want %d",
			len(knownCheckpointOptions), len(expectedOptions))
	}
}

func TestSupportedCheckpointOptions(t *testing.T) {
	// Verify supported options are properly defined with help text
	expectedOptions := map[string]struct {
		optType OptionType
		help    string
	}{
		"leave-running": {
			optType: OptionTypeBool,
			help:    "leave container(s) in running state after checkpointing",
		},
	}

	for name, expected := range expectedOptions {
		opt, exists := SupportedCheckpointOptions[name]
		if !exists {
			t.Errorf("expected option %q to exist in SupportedCheckpointOptions", name)
			continue
		}
		if opt.Type != expected.optType {
			t.Errorf("option %q has type %v, want %v", name, opt.Type, expected.optType)
		}
		if opt.Help != expected.help {
			t.Errorf("option %q has help %q, want %q", name, opt.Help, expected.help)
		}
	}

	if len(SupportedCheckpointOptions) != len(expectedOptions) {
		t.Errorf("SupportedCheckpointOptions has %d entries, want %d",
			len(SupportedCheckpointOptions), len(expectedOptions))
	}

	// Verify all supported options are also in knownCheckpointOptions
	for name, supported := range SupportedCheckpointOptions {
		known, exists := knownCheckpointOptions[name]
		if !exists {
			t.Errorf("supported option %q not found in knownCheckpointOptions", name)
			continue
		}
		if known.Type != supported.Type {
			t.Errorf("option %q type mismatch: supported=%v, known=%v",
				name, supported.Type, known.Type)
		}
	}
}
