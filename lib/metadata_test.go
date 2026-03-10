// SPDX-License-Identifier: Apache-2.0

package metadata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCheckpointPodOptions(t *testing.T) {
	tests := []struct {
		name            string
		podOptions      CheckpointedPodOptions
		wantVersion     int
		wantContLen     int
		wantAnnotations map[string]string
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
		{
			name: "with annotations",
			podOptions: CheckpointedPodOptions{
				Version: 1,
				Containers: map[string]string{
					"test": "test-container",
				},
				Annotations: map[string]string{
					CheckpointAnnotationEngine:        "podman",
					CheckpointAnnotationEngineVersion: "4.0.0",
					CheckpointAnnotationPod:           "test-pod",
				},
			},
			wantVersion: 1,
			wantContLen: 1,
			wantAnnotations: map[string]string{
				CheckpointAnnotationEngine:        "podman",
				CheckpointAnnotationEngineVersion: "4.0.0",
				CheckpointAnnotationPod:           "test-pod",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if _, err := WriteJSONFile(&tt.podOptions, tmpDir, PodOptionsFile); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			podOptions, _, err := ReadCheckpointPodOptions(tmpDir)
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

			if tt.wantAnnotations != nil {
				if len(podOptions.Annotations) != len(tt.wantAnnotations) {
					t.Errorf("expected %d annotations, got %d", len(tt.wantAnnotations), len(podOptions.Annotations))
				}
				for k, v := range tt.wantAnnotations {
					if podOptions.Annotations[k] != v {
						t.Errorf("expected annotation %s=%s, got %s", k, v, podOptions.Annotations[k])
					}
				}
			}
		})
	}
}

func TestReadCheckpointPodOptionsFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, _, err := ReadCheckpointPodOptions(tmpDir)
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}

func TestReadContainerCheckpointNetworkStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     PodmanNetworkStatus
		wantIP     string
		wantMAC    string
		wantIPNet  string
		wantGW     string
		wantIfaces int
	}{
		{
			name: "single network single interface",
			status: PodmanNetworkStatus{
				"podman": PodmanNetworkResult{
					Interfaces: map[string]PodmanNetworkInterface{
						"eth0": {
							Subnets: []PodmanNetworkSubnet{
								{IPNet: "10.88.0.9/16", Gateway: "10.88.0.1"},
							},
							MacAddress: "f2:99:8d:fb:5a:57",
						},
					},
				},
			},
			wantIP:     "10.88.0.9/16",
			wantMAC:    "f2:99:8d:fb:5a:57",
			wantGW:     "10.88.0.1",
			wantIfaces: 1,
		},
		{
			name: "multiple subnets",
			status: PodmanNetworkStatus{
				"podman": PodmanNetworkResult{
					Interfaces: map[string]PodmanNetworkInterface{
						"eth0": {
							Subnets: []PodmanNetworkSubnet{
								{IPNet: "10.88.0.5/16", Gateway: "10.88.0.1"},
								{IPNet: "fd00::5/64", Gateway: "fd00::1"},
							},
							MacAddress: "aa:bb:cc:dd:ee:ff",
						},
					},
				},
			},
			wantIP:     "10.88.0.5/16",
			wantMAC:    "aa:bb:cc:dd:ee:ff",
			wantGW:     "10.88.0.1",
			wantIfaces: 1,
		},
		{
			name: "multiple networks multiple interfaces",
			status: PodmanNetworkStatus{
				"net1": PodmanNetworkResult{
					Interfaces: map[string]PodmanNetworkInterface{
						"eth0": {
							Subnets: []PodmanNetworkSubnet{
								{IPNet: "10.89.0.2/24", Gateway: "10.89.0.1"},
							},
							MacAddress: "32:ba:b8:45:bc:84",
						},
					},
				},
				"net2": PodmanNetworkResult{
					Interfaces: map[string]PodmanNetworkInterface{
						"eth1": {
							Subnets: []PodmanNetworkSubnet{
								{IPNet: "10.89.1.2/24", Gateway: "10.89.1.1"},
							},
							MacAddress: "5e:b7:fe:ee:e0:d8",
						},
					},
				},
			},
			wantIfaces: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if _, err := WriteJSONFile(&tt.status, tmpDir, NetworkStatusFile); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			networkStatus, _, err := ReadContainerCheckpointNetworkStatus(tmpDir)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			for _, network := range *networkStatus {
				if len(network.Interfaces) != tt.wantIfaces {
					t.Errorf("expected %d interfaces, got %d", tt.wantIfaces, len(network.Interfaces))
				}
				for _, iface := range network.Interfaces {
					if tt.wantMAC != "" && iface.MacAddress != tt.wantMAC {
						t.Errorf("expected MAC %s, got %s", tt.wantMAC, iface.MacAddress)
					}
					if len(iface.Subnets) > 0 {
						if tt.wantIP != "" && iface.Subnets[0].IPNet != tt.wantIP {
							t.Errorf("expected IP %s, got %s", tt.wantIP, iface.Subnets[0].IPNet)
						}
						if tt.wantGW != "" && iface.Subnets[0].Gateway != tt.wantGW {
							t.Errorf("expected gateway %s, got %s", tt.wantGW, iface.Subnets[0].Gateway)
						}
					}
				}
			}
		})
	}
}

func TestReadContainerCheckpointNetworkStatusFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, _, err := ReadContainerCheckpointNetworkStatus(tmpDir)
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}

func TestReadContainerCheckpointNetworkStatusBrokenFile(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, NetworkStatusFile), []byte("not-valid-json"), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, _, err := ReadContainerCheckpointNetworkStatus(tmpDir)
	if err == nil {
		t.Errorf("expected error for broken file, got nil")
	}
}
