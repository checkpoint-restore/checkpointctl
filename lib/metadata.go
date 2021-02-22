// SPDX-License-Identifier: Apache-2.0

package metadata

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"time"

	cnitypes "github.com/containernetworking/cni/pkg/types/current"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

type CheckpointedPods struct {
	PodUID                 string                   `json:"io.kubernetes.pod.uid,omitempty"`
	ID                     string                   `json:"SandboxID,omitempty"`
	Name                   string                   `json:"io.kubernetes.pod.name,omitempty"`
	TerminationGracePeriod int64                    `json:"io.kubernetes.pod.terminationGracePeriod,omitempty"`
	Namespace              string                   `json:"io.kubernetes.pod.namespace,omitempty"`
	ConfigSource           string                   `json:"kubernetes.io/config.source,omitempty"`
	ConfigSeen             string                   `json:"kubernetes.io/config.seen,omitempty"`
	Manager                string                   `json:"io.container.manager,omitempty"`
	Containers             []CheckpointedContainers `json:"Containers"`
	HostIP                 string                   `json:"hostIP,omitempty"`
	PodIP                  string                   `json:"podIP,omitempty"`
	PodIPs                 []string                 `json:"podIPs,omitempty"`
}

type CheckpointedContainers struct {
	Name                      string `json:"io.kubernetes.container.name,omitempty"`
	ID                        string `json:"id,omitempty"`
	TerminationMessagePath    string `json:"io.kubernetes.container.terminationMessagePath,omitempty"`
	TerminationMessagePolicy  string `json:"io.kubernetes.container.terminationMessagePolicy,omitempty"`
	RestartCounter            int32  `json:"io.kubernetes.container.restartCount,omitempty"`
	TerminationMessagePathUID string `json:"terminationMessagePathUID,omitempty"`
	Image                     string `json:"Image"`
}

type CheckpointMetadata struct {
	Version          int `json:"version"`
	CheckpointedPods []CheckpointedPods
}

const (
	// kubelet archive
	CheckpointedPodsFile = "checkpointed.pods"
	// container archive
	ConfigDumpFile      = "config.dump"
	SpecDumpFile        = "spec.dump"
	NetworkStatusFile   = "network.status"
	CheckpointDirectory = "checkpoint"
	RootFsDiffTar       = "rootfs-diff.tar"
	// pod archive
	PodOptionsFile = "pod.options"
)

type CheckpointType int

const (
	// The checkpoint archive contains a kubelet checkpoint
	// One or multiple pods and kubelet metadata (checkpointed.pods)
	Kubelet CheckpointType = iota
	// The checkpoint archive contains one pod including one or multiple containers
	Pod
	// The checkpoint archive contains a single container
	Container
	Unknown
)

// This is a reduced copy of what Podman uses to store checkpoint metadata
type ContainerConfig struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	RootfsImageName string    `json:"rootfsImageName,omitempty"`
	OCIRuntime      string    `json:"runtime,omitempty"`
	CreatedTime     time.Time `json:"createdTime"`
}

func DetectCheckpointArchiveType(checkpointDirectory string) (error, CheckpointType) {
	_, err := os.Stat(filepath.Join(checkpointDirectory, CheckpointedPodsFile))
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "Failed to access %q\n", CheckpointedPodsFile), Unknown
	}
	if os.IsNotExist(err) {
		return nil, Container
	}

	return nil, Kubelet
}

func ReadContainerCheckpointSpecDump(checkpointDirectory string) (error, *spec.Spec, string) {
	var specDump spec.Spec
	err, specDumpFile := ReadJSONFile(&specDump, checkpointDirectory, SpecDumpFile)

	return err, &specDump, specDumpFile
}

func ReadContainerCheckpointConfigDump(checkpointDirectory string) (error, *ContainerConfig, string) {
	var containerConfig ContainerConfig
	err, configDumpFile := ReadJSONFile(&containerConfig, checkpointDirectory, ConfigDumpFile)

	return err, &containerConfig, configDumpFile
}

func ReadContainerCheckpointNetworkStatus(checkpointDirectory string) (error, []*cnitypes.Result, string) {
	var networkStatus []*cnitypes.Result
	err, networkStatusFile := ReadJSONFile(&networkStatus, checkpointDirectory, NetworkStatusFile)

	return err, networkStatus, networkStatusFile
}

func ReadKubeletCheckpoints(checkpointsDirectory string) (error, *CheckpointMetadata, string) {
	var checkpointMetadata CheckpointMetadata
	err, checkpointMetadataPath := ReadJSONFile(&checkpointMetadata, checkpointsDirectory, CheckpointedPodsFile)

	return err, &checkpointMetadata, checkpointMetadataPath
}

func GetIPFromNetworkStatus(networkStatus []*cnitypes.Result) net.IP {
	if len(networkStatus) == 0 {
		return nil
	}
	// Take the first IP address
	if len(networkStatus[0].IPs) == 0 {
		return nil
	}
	IP := networkStatus[0].IPs[0].Address.IP

	return IP
}

func GetMACFromNetworkStatus(networkStatus []*cnitypes.Result) net.HardwareAddr {
	if len(networkStatus) == 0 {
		return nil
	}
	// Take the first device with a defined sandbox
	if len(networkStatus[0].Interfaces) == 0 {
		return nil
	}
	var MAC net.HardwareAddr
	MAC = nil
	for _, n := range networkStatus[0].Interfaces {
		if n.Sandbox != "" {
			MAC, _ = net.ParseMAC(n.Mac)
			break
		}
	}

	return MAC
}

// WriteJSONFile marshalls and writes the given data to a JSON file
func WriteJSONFile(v interface{}, dir, file string) (error, string) {
	fileJSON, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "Error marshalling JSON"), ""
	}
	file = filepath.Join(dir, file)
	if err := ioutil.WriteFile(file, fileJSON, 0o644); err != nil {
		return errors.Wrapf(err, "Error writing to %q", file), ""
	}

	return nil, file
}

func ReadJSONFile(v interface{}, dir, file string) (error, string) {
	file = filepath.Join(dir, file)
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", file), ""
	}
	if err = json.Unmarshal(content, v); err != nil {
		return errors.Wrapf(err, "failed to unmarshal %s", file), ""
	}

	return nil, file
}

func WriteKubeletCheckpointsMetadata(checkpointMetadata *CheckpointMetadata, dir string) error {
	err, _ := WriteJSONFile(checkpointMetadata, dir, CheckpointedPodsFile)
	return err
}

func ByteToString(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB",
		float64(b)/float64(div), "KMGTPE"[exp])
}
