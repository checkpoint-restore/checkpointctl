package internal

import (
	"encoding/json"
	"fmt"
	"os"
)

// PodmanNetworkStatus represents the network status structure for Podman
type PodmanNetworkStatus struct {
	Podman struct {
		Interfaces map[string]struct {
			Subnets []struct {
				IPNet   string `json:"ipnet"`
				Gateway string `json:"gateway"`
			} `json:"subnets"`
			MacAddress string `json:"mac_address"`
		} `json:"interfaces"`
	} `json:"podman"`
}

// getPodmanNetworkInfo reads and parses the network.status file from a Podman checkpoint
func getPodmanNetworkInfo(networkStatusFile string) (string, string, error) {
	data, err := os.ReadFile(networkStatusFile)
	if err != nil {
		// Return empty strings if file doesn't exist or can't be read
		// This maintains compatibility with containers that don't have network info
		return "", "", nil
	}

	var status PodmanNetworkStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return "", "", fmt.Errorf("failed to parse network status: %w", err)
	}

	// Get the first interface's information
	// Most containers will have a single interface (eth0)
	for _, info := range status.Podman.Interfaces {
		if len(info.Subnets) > 0 {
			return info.Subnets[0].IPNet, info.MacAddress, nil
		}
	}

	return "", "", nil
}