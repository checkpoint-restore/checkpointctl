package internal

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"testing"
)

func TestRemapEnvSlice(t *testing.T) {
	tests := []struct {
		name     string
		env      []any
		oldPort  string
		newPort  string
		expected []any
	}{
		{
			name:     "basic port replacement",
			env:      []any{"A=1", "PORT=5000", "B=2"},
			oldPort:  "5000",
			newPort:  "9999",
			expected: []any{"A=1", "PORT=9999", "B=2"},
		},
		{
			name:     "non-string element preserved",
			env:      []any{"PORT=5000", 123},
			oldPort:  "5000",
			newPort:  "9999",
			expected: []any{"PORT=9999", 123},
		},
		{
			name:     "no match - no change",
			env:      []any{"PORT=8080", "FOO=bar"},
			oldPort:  "5000",
			newPort:  "9999",
			expected: []any{"PORT=8080", "FOO=bar"},
		},
		{
			name:     "multiple PORT entries",
			env:      []any{"PORT=5000", "OTHER=x", "PORT=5000"},
			oldPort:  "5000",
			newPort:  "9999",
			expected: []any{"PORT=9999", "OTHER=x", "PORT=9999"},
		},
		{
			name:     "empty slice",
			env:      []any{},
			oldPort:  "5000",
			newPort:  "9999",
			expected: []any{},
		},
		{
			name:     "PORT as substring not replaced",
			env:      []any{"MYPORT=5000", "EXPORT=5000"},
			oldPort:  "5000",
			newPort:  "9999",
			expected: []any{"MYPORT=5000", "EXPORT=5000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := remapEnvSlice(tt.env, tt.oldPort, tt.newPort)
			if len(got) != len(tt.expected) {
				t.Errorf("Expected env length %d, got %d", len(tt.expected), len(got))
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Errorf("env[%d]: expected %v, got %v", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestRemapEnvRecursive(t *testing.T) {
	tests := []struct {
		name    string
		obj     any
		oldPort string
		newPort string
		check   func(t *testing.T, obj any)
	}{
		{
			name: "nested env remapping",
			obj: map[string]any{
				"env": []any{"PORT=5000", "FOO=bar"},
				"process": map[string]any{
					"env": []any{"A=1", "PORT=5000"},
				},
			},
			oldPort: "5000",
			newPort: "9999",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				rootEnv := m["env"].([]any)
				if rootEnv[0] != "PORT=9999" {
					t.Errorf("Expected root env PORT to be remapped, got %v", rootEnv[0])
				}
				nestedEnv := m["process"].(map[string]any)["env"].([]any)
				if nestedEnv[1] != "PORT=9999" {
					t.Errorf("Expected nested env PORT to be remapped, got %v", nestedEnv[1])
				}
			},
		},
		{
			name: "deeply nested env",
			obj: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"env": []any{"PORT=5000"},
					},
				},
			},
			oldPort: "5000",
			newPort: "9999",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				env := m["level1"].(map[string]any)["level2"].(map[string]any)["env"].([]any)
				if env[0] != "PORT=9999" {
					t.Errorf("Expected deeply nested PORT to be remapped, got %v", env[0])
				}
			},
		},
		{
			name: "env in array elements",
			obj: map[string]any{
				"containers": []any{
					map[string]any{"env": []any{"PORT=5000"}},
					map[string]any{"env": []any{"PORT=5000"}},
				},
			},
			oldPort: "5000",
			newPort: "9999",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				containers := m["containers"].([]any)
				env0 := containers[0].(map[string]any)["env"].([]any)
				env1 := containers[1].(map[string]any)["env"].([]any)
				if env0[0] != "PORT=9999" || env1[0] != "PORT=9999" {
					t.Errorf("Expected all container envs to be remapped")
				}
			},
		},
		{
			name:    "no env field - no error",
			obj:     map[string]any{"other": "data"},
			oldPort: "5000",
			newPort: "9999",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				if m["other"] != "data" {
					t.Errorf("Expected object to remain unchanged")
				}
			},
		},
		{
			name:    "non-map input - no panic",
			obj:     "not a map",
			oldPort: "5000",
			newPort: "9999",
			check:   func(t *testing.T, obj any) {},
		},
		{
			name: "env is not array - no panic",
			obj: map[string]any{
				"env": "not an array",
			},
			oldPort: "5000",
			newPort: "9999",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				if m["env"] != "not an array" {
					t.Errorf("Expected env to remain unchanged")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remapEnvRecursive(tt.obj, tt.oldPort, tt.newPort)
			tt.check(t, tt.obj)
		})
	}
}

func TestRemapPortMappings(t *testing.T) {
	tests := []struct {
		name    string
		obj     any
		oldPort string
		newPort string
		check   func(t *testing.T, obj any)
	}{
		{
			name: "basic port mapping",
			obj: map[string]any{
				"newPortMappings": []any{
					map[string]any{"container_port": float64(5000)},
					map[string]any{"container_port": float64(6000)},
				},
			},
			oldPort: "5000",
			newPort: "9999",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				mappings := m["newPortMappings"].([]any)
				if mappings[0].(map[string]any)["container_port"].(float64) != 9999 {
					t.Errorf("Expected first port to be 9999")
				}
				if mappings[1].(map[string]any)["container_port"].(float64) != 6000 {
					t.Errorf("Expected second port to remain 6000")
				}
			},
		},
		{
			name: "port with additional fields",
			obj: map[string]any{
				"newPortMappings": []any{
					map[string]any{
						"host_ip":        "",
						"container_port": float64(8000),
						"host_port":      float64(8080),
						"range":          float64(1),
						"protocol":       "tcp",
					},
				},
			},
			oldPort: "8000",
			newPort: "9000",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				mappings := m["newPortMappings"].([]any)
				mapping := mappings[0].(map[string]any)
				if mapping["container_port"].(float64) != 9000 {
					t.Errorf("Expected container_port to be 9000")
				}
				if mapping["host_port"].(float64) != 8080 {
					t.Errorf("Expected host_port to remain 8080")
				}
				if mapping["protocol"] != "tcp" {
					t.Errorf("Expected other fields to be preserved")
				}
			},
		},
		{
			name:    "no newPortMappings field",
			obj:     map[string]any{"other": "data"},
			oldPort: "5000",
			newPort: "9999",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				if m["other"] != "data" {
					t.Errorf("Expected object to remain unchanged")
				}
			},
		},
		{
			name: "empty newPortMappings array",
			obj: map[string]any{
				"newPortMappings": []any{},
			},
			oldPort: "5000",
			newPort: "9999",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				mappings := m["newPortMappings"].([]any)
				if len(mappings) != 0 {
					t.Errorf("Expected empty array to remain empty")
				}
			},
		},
		{
			name:    "non-map input",
			obj:     "not a map",
			oldPort: "5000",
			newPort: "9999",
			check:   func(t *testing.T, obj any) {},
		},
		{
			name: "newPortMappings not an array",
			obj: map[string]any{
				"newPortMappings": "not an array",
			},
			oldPort: "5000",
			newPort: "9999",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				if m["newPortMappings"] != "not an array" {
					t.Errorf("Expected field to remain unchanged")
				}
			},
		},
		{
			name: "mapping without container_port field",
			obj: map[string]any{
				"newPortMappings": []any{
					map[string]any{"host_port": float64(8080)},
				},
			},
			oldPort: "5000",
			newPort: "9999",
			check: func(t *testing.T, obj any) {
				m := obj.(map[string]any)
				mappings := m["newPortMappings"].([]any)
				mapping := mappings[0].(map[string]any)
				if _, exists := mapping["container_port"]; exists {
					t.Errorf("Expected no container_port to be added")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remapPortMappings(tt.obj, tt.oldPort, tt.newPort)
			tt.check(t, tt.obj)
		})
	}
}

func TestRemapConfigDump(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		oldPort string
		newPort string
		wantErr bool
		check   func(t *testing.T, output []byte)
	}{
		{
			name: "full config with ports and env",
			input: `{
				"newPortMappings":[{"container_port":5000},{"container_port":7000}],
				"env":["FOO=bar","PORT=5000"],
				"nested":{"env":["PORT=5000","X=1"]}
			}`,
			oldPort: "5000",
			newPort: "9999",
			wantErr: false,
			check: func(t *testing.T, output []byte) {
				var got map[string]any
				if err := json.Unmarshal(output, &got); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}

				ports := got["newPortMappings"].([]any)
				if ports[0].(map[string]any)["container_port"].(float64) != 9999 {
					t.Errorf("Expected first port to be 9999")
				}
				if ports[1].(map[string]any)["container_port"].(float64) != 7000 {
					t.Errorf("Expected second port to remain 7000")
				}

				env := got["env"].([]any)
				if env[1] != "PORT=9999" {
					t.Errorf("Expected PORT env to be remapped")
				}

				nestedEnv := got["nested"].(map[string]any)["env"].([]any)
				if nestedEnv[0] != "PORT=9999" {
					t.Errorf("Expected nested PORT env to be remapped")
				}
			},
		},
		{
			name:    "empty config",
			input:   "{}",
			oldPort: "5000",
			newPort: "9999",
			wantErr: false,
			check: func(t *testing.T, output []byte) {
				var got map[string]any
				if err := json.Unmarshal(output, &got); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}
			},
		},
		{
			name:    "invalid JSON",
			input:   "{invalid json",
			oldPort: "5000",
			newPort: "9999",
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hdr := &tar.Header{Name: "config.dump", Size: int64(len(tt.input))}
			newHdr, out, err := remapConfigDump(hdr, bytes.NewReader([]byte(tt.input)), tt.oldPort, tt.newPort)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if newHdr.Size != int64(len(out)) {
				t.Errorf("Expected header size %d, got %d", len(out), newHdr.Size)
			}

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func TestRemapSpecDump(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		oldPort string
		newPort string
		wantErr bool
		check   func(t *testing.T, output []byte)
	}{
		{
			name:    "basic spec with env",
			input:   `{"process":{"env":["PORT=5000","FOO=bar"]}}`,
			oldPort: "5000",
			newPort: "9999",
			wantErr: false,
			check: func(t *testing.T, output []byte) {
				var got map[string]any
				if err := json.Unmarshal(output, &got); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}
				env := got["process"].(map[string]any)["env"].([]any)
				if env[0] != "PORT=9999" {
					t.Errorf("Expected PORT to be remapped, got %v", env[0])
				}
				if env[1] != "FOO=bar" {
					t.Errorf("Expected FOO to remain unchanged")
				}
			},
		},
		{
			name:    "spec without process",
			input:   `{"other":"field"}`,
			oldPort: "5000",
			newPort: "9999",
			wantErr: false,
			check: func(t *testing.T, output []byte) {
				var got map[string]any
				if err := json.Unmarshal(output, &got); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}
				if got["other"] != "field" {
					t.Errorf("Expected spec to remain unchanged")
				}
			},
		},
		{
			name:    "spec with process but no env",
			input:   `{"process":{"user":{"uid":0}}}`,
			oldPort: "5000",
			newPort: "9999",
			wantErr: false,
			check: func(t *testing.T, output []byte) {
				var got map[string]any
				if err := json.Unmarshal(output, &got); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}
				process := got["process"].(map[string]any)
				if process["user"].(map[string]any)["uid"].(float64) != 0 {
					t.Errorf("Expected process to remain unchanged")
				}
			},
		},
		{
			name:    "spec with empty env array",
			input:   `{"process":{"env":[]}}`,
			oldPort: "5000",
			newPort: "9999",
			wantErr: false,
			check: func(t *testing.T, output []byte) {
				var got map[string]any
				if err := json.Unmarshal(output, &got); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}
				env := got["process"].(map[string]any)["env"].([]any)
				if len(env) != 0 {
					t.Errorf("Expected empty env to remain empty")
				}
			},
		},
		{
			name:    "invalid JSON",
			input:   "not json",
			oldPort: "5000",
			newPort: "9999",
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hdr := &tar.Header{Name: "spec.dump", Size: int64(len(tt.input))}
			newHdr, out, err := remapSpecDump(hdr, bytes.NewReader([]byte(tt.input)), tt.oldPort, tt.newPort)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if newHdr.Size != int64(len(out)) {
				t.Errorf("Expected header size %d, got %d", len(out), newHdr.Size)
			}

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}
