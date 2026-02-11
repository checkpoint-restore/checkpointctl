// SPDX-License-Identifier: Apache-2.0

package metadata

import (
	"testing"
)

func TestReadContainerCheckpointPodOptions(t *testing.T) {
	tests := []struct {
		name        string
		podOptions  CheckpointedPodOptions
		wantVersion int
		wantContLen int
	}{
		{
			name: "valid pod options",
			podOptions: CheckpointedPodOptions{
				Version: 1,
				Containers: map[string]string{
					"short-name": "full-container-name",
					"another":    "another-full-name",
				},
			},
			wantVersion: 1,
			wantContLen: 2,
		},
		{
			name: "empty containers",
			podOptions: CheckpointedPodOptions{
				Version:    2,
				Containers: map[string]string{},
			},
			wantVersion: 2,
			wantContLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if _, err := WriteJSONFile(&tt.podOptions, tmpDir, PodOptionsFile); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			podOptions, _, err := ReadContainerCheckpointPodOptions(tmpDir)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if podOptions.Version != tt.wantVersion {
				t.Errorf("expected version %d, got %d", tt.wantVersion, podOptions.Version)
			}

			if len(podOptions.Containers) != tt.wantContLen {
				t.Errorf("expected %d containers, got %d", tt.wantContLen, len(podOptions.Containers))
			}
		})
	}
}

func TestReadContainerCheckpointPodOptionsFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, _, err := ReadContainerCheckpointPodOptions(tmpDir)
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}
