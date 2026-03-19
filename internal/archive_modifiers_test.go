package internal

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"testing"
)

func TestRemapEnvSlice(t *testing.T) {
	env := []any{"A=1", "PORT=5000", 123}

	got := remapEnvSlice(env, "5000", "9999")
	expected := []any{"A=1", "PORT=9999", 123}

	if len(got) != len(expected) {
		t.Errorf("Expected env length %d, got %d", len(expected), len(got))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Expected env[%d]=%v, got %v", i, expected[i], got[i])
		}
	}
}

func TestRemapEnvRecursive(t *testing.T) {
	obj := map[string]any{
		"env": []any{"PORT=5000", "FOO=bar"},
		"process": map[string]any{
			"env": []any{"A=1", "PORT=5000"},
		},
	}

	remapEnvRecursive(obj, "5000", "9999")

	rootEnv := obj["env"].([]any)
	if rootEnv[0] != "PORT=9999" {
		t.Errorf("Expected root env PORT to be remapped, got %v", rootEnv[0])
	}

	nestedEnv := obj["process"].(map[string]any)["env"].([]any)
	if nestedEnv[1] != "PORT=9999" {
		t.Errorf("Expected nested env PORT to be remapped, got %v", nestedEnv[1])
	}
}

func TestRemapPortMappings(t *testing.T) {
	obj := map[string]any{
		"newPortMappings": []any{
			map[string]any{"container_port": float64(5000)},
			map[string]any{"container_port": float64(6000)},
		},
		"nested": map[string]any{
			"portMappings": []any{
				map[string]any{"container_port": float64(5000)},
			},
		},
	}

	remapPortMappings(obj, "5000", "9999")

	top := obj["newPortMappings"].([]any)
	if top[0].(map[string]any)["container_port"].(float64) != 9999 {
		t.Errorf("Expected top-level container_port to be 9999")
	}
	if top[1].(map[string]any)["container_port"].(float64) != 6000 {
		t.Errorf("Expected non-target top-level container_port to stay 6000")
	}

	nested := obj["nested"].(map[string]any)["portMappings"].([]any)
	if nested[0].(map[string]any)["container_port"].(float64) != 9999 {
		t.Errorf("Expected nested container_port to be 9999")
	}
}

func TestRemapConfigDump(t *testing.T) {
	input := []byte(`{
		"newPortMappings":[{"container_port":5000},{"container_port":7000}],
		"env":["FOO=bar","PORT=5000"],
		"nested":{"env":["PORT=5000","X=1"]}
	}`)
	hdr := &tar.Header{Name: "config.dump", Size: int64(len(input))}

	newHdr, out, err := remapConfigDump(hdr, bytes.NewReader(input), "5000", "9999")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if newHdr.Size != int64(len(out)) {
		t.Errorf("Expected header size %d, got %d", len(out), newHdr.Size)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("Failed to unmarshal output JSON: %v", err)
	}

	ports := got["newPortMappings"].([]any)
	if ports[0].(map[string]any)["container_port"].(float64) != 9999 {
		t.Errorf("Expected first mapped port to be 9999")
	}
	if ports[1].(map[string]any)["container_port"].(float64) != 7000 {
		t.Errorf("Expected non-target mapped port to remain 7000")
	}

	env := got["env"].([]any)
	if env[1] != "PORT=9999" {
		t.Errorf("Expected root PORT env to be remapped, got %v", env[1])
	}

	nestedEnv := got["nested"].(map[string]any)["env"].([]any)
	if nestedEnv[0] != "PORT=9999" {
		t.Errorf("Expected nested PORT env to be remapped, got %v", nestedEnv[0])
	}
}

func TestRemapSpecDump(t *testing.T) {
	input := []byte(`{"process":{"env":["PORT=5000","FOO=bar"]}}`)
	hdr := &tar.Header{Name: "spec.dump", Size: int64(len(input))}

	newHdr, out, err := remapSpecDump(hdr, bytes.NewReader(input), "5000", "9999")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if newHdr.Size != int64(len(out)) {
		t.Errorf("Expected header size %d, got %d", len(out), newHdr.Size)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("Failed to unmarshal output JSON: %v", err)
	}

	env := got["process"].(map[string]any)["env"].([]any)
	if env[0] != "PORT=9999" {
		t.Errorf("Expected PORT env to be remapped, got %v", env[0])
	}
}
