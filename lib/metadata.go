// SPDX-License-Identifier: Apache-2.0

package metadata

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

type CheckpointedContainer struct {
	Name                      string `json:"io.kubernetes.container.name,omitempty"`
	ID                        string `json:"id,omitempty"`
	TerminationMessagePath    string `json:"io.kubernetes.container.terminationMessagePath,omitempty"`
	TerminationMessagePolicy  string `json:"io.kubernetes.container.terminationMessagePolicy,omitempty"`
	RestartCounter            int32  `json:"io.kubernetes.container.restartCount,omitempty"`
	TerminationMessagePathUID string `json:"terminationMessagePathUID,omitempty"`
	Image                     string `json:"Image"`
}

const (
	// container archive
	ConfigDumpFile             = "config.dump"
	SpecDumpFile               = "spec.dump"
	NetworkStatusFile          = "network.status"
	CheckpointDirectory        = "checkpoint"
	CheckpointVolumesDirectory = "volumes"
	DevShmCheckpointTar        = "devshm-checkpoint.tar"
	RootFsDiffTar              = "rootfs-diff.tar"
	DeletedFilesFile           = "deleted.files"
	DumpLogFile                = "dump.log"
	RestoreLogFile             = "restore.log"
	// containerd only
	StatusFile = "status"
)

// This is a reduced copy of what Podman uses to store checkpoint metadata
type ContainerConfig struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	RootfsImageName string    `json:"rootfsImageName,omitempty"`
	OCIRuntime      string    `json:"runtime,omitempty"`
	CreatedTime     time.Time `json:"createdTime"`
}

// This is metadata stored inside of a Pod checkpoint archive
type CheckpointedPodOptions struct {
	Version      int      `json:"version"`
	Containers   []string `json:"containers,omitempty"`
	MountLabel   string   `json:"mountLabel"`
	ProcessLabel string   `json:"processLabel"`
}

// This is metadata stored inside of Pod checkpoint archive
type PodSandboxConfig struct {
	Metadata SandboxMetadta `json:"metadata"`
	Hostname string         `json:"hostname"`
}

type SandboxMetadta struct {
	Name      string `json:"name"`
	UID       string `json:"uid"`
	Namespace string `json:"namespace"`
}

type ContainerdStatus struct {
	CreatedAt  int64
	StartedAt  int64
	FinishedAt int64
	ExitCode   int32
	Pid        uint32
	Reason     string
	Message    string
}

func ReadContainerCheckpointSpecDump(checkpointDirectory string) (*spec.Spec, string, error) {
	var specDump spec.Spec
	specDumpFile, err := ReadJSONFile(&specDump, checkpointDirectory, SpecDumpFile)

	return &specDump, specDumpFile, err
}

func ReadContainerCheckpointConfigDump(checkpointDirectory string) (*ContainerConfig, string, error) {
	var containerConfig ContainerConfig
	configDumpFile, err := ReadJSONFile(&containerConfig, checkpointDirectory, ConfigDumpFile)

	return &containerConfig, configDumpFile, err
}

func ReadContainerCheckpointDeletedFiles(checkpointDirectory string) ([]string, string, error) {
	var deletedFiles []string
	deletedFilesFile, err := ReadJSONFile(&deletedFiles, checkpointDirectory, DeletedFilesFile)

	return deletedFiles, deletedFilesFile, err
}

func ReadContainerCheckpointStatusFile(checkpointDirectory string) (*ContainerdStatus, string, error) {
	var containerdStatus ContainerdStatus
	statusFile, err := ReadJSONFile(&containerdStatus, checkpointDirectory, StatusFile)

	return &containerdStatus, statusFile, err
}

// WriteJSONFile marshalls and writes the given data to a JSON file
func WriteJSONFile(v interface{}, dir, file string) (string, error) {
	fileJSON, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", errors.Wrapf(err, "Error marshalling JSON")
	}
	file = filepath.Join(dir, file)
	if err := ioutil.WriteFile(file, fileJSON, 0o600); err != nil {
		return "", errors.Wrapf(err, "Error writing to %q", file)
	}

	return file, nil
}

func ReadJSONFile(v interface{}, dir, file string) (string, error) {
	file = filepath.Join(dir, file)
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read %s", file)
	}
	if err = json.Unmarshal(content, v); err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal %s", file)
	}

	return file, nil
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
