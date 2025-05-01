package internal

import (
	"os"
	"path/filepath"
	"testing"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
)

func TestGetPodmanNetworkInfo(t *testing.T) {
	// Test case 1: Valid network status file
	networkStatus := `{
		"podman": {
			"interfaces": {
				"eth0": {
					"subnets": [
						{
							"ipnet": "10.88.0.9/16",
							"gateway": "10.88.0.1"
						}
					],
					"mac_address": "f2:99:8d:fb:5a:57"
				}
			}
		}
	}`

	networkStatusFile := filepath.Join(t.TempDir(), metadata.NetworkStatusFile)
	if err := os.WriteFile(networkStatusFile, []byte(networkStatus), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	ip, mac, err := getPodmanNetworkInfo(networkStatusFile)
	if err != nil {
		t.Errorf("getPodmanNetworkInfo failed: %v", err)
	}

	expectedIP := "10.88.0.9/16"
	expectedMAC := "f2:99:8d:fb:5a:57"

	if ip != expectedIP {
		t.Errorf("Expected IP %s, got %s", expectedIP, ip)
	}
	if mac != expectedMAC {
		t.Errorf("Expected MAC %s, got %s", expectedMAC, mac)
	}

	// Test case 2: Missing network status file
	nonExistentFile := filepath.Join(t.TempDir(), metadata.NetworkStatusFile)
	ip, mac, err = getPodmanNetworkInfo(nonExistentFile)
	if err != nil {
		t.Errorf("getPodmanNetworkInfo with missing file should not return error, got: %v", err)
	}
	if ip != "" || mac != "" {
		t.Errorf("Expected empty IP and MAC for missing file, got IP=%s, MAC=%s", ip, mac)
	}

	// Test case 3: Invalid JSON
	invalidJSONFile := filepath.Join(t.TempDir(), metadata.NetworkStatusFile)
	if err := os.WriteFile(invalidJSONFile, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	ip, mac, err = getPodmanNetworkInfo(invalidJSONFile)
	if err == nil {
		t.Error("getPodmanNetworkInfo should fail with invalid JSON")
	}
}