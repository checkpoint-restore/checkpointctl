package internal

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
)

type ChkptConfig struct {
	Namespace        string
	Pod              string
	Container        string
	ContainerManager string
	Timestamp        time.Time
}

func ExtractConfigDump(checkpointPath string) (*ChkptConfig, error) {
	tempDir, err := os.MkdirTemp("", "extracted-checkpoint")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	filesToExtract := []string{"spec.dump", "config.dump"}
	if err := UntarFiles(checkpointPath, tempDir, filesToExtract); err != nil {
		log.Printf("Error extracting files from archive %s: %v\n", checkpointPath, err)
		return nil, err
	}

	specDumpPath := filepath.Join(tempDir, "spec.dump")
	specContent, err := os.ReadFile(specDumpPath)
	if err != nil {
		log.Printf("Error reading spec.dump file: %v\n", err)
		return nil, err
	}

	configDumpPath := filepath.Join(tempDir, "config.dump")
	configContent, err := os.ReadFile(configDumpPath)
	if err != nil {
		log.Printf("Error reading config.dump file: %v\n", err)
		return nil, err
	}

	return extractConfigDumpContent(configContent, specContent)
}

func extractConfigDumpContent(configContent []byte, specContent []byte) (*ChkptConfig, error) {
	var spec metadata.Spec
	var config metadata.ContainerConfig

	if err := json.Unmarshal(configContent, &config); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(specContent, &spec); err != nil {
		return nil, err
	}

	namespace := spec.Annotations["io.kubernetes.pod.namespace"]
	timestamp := config.CheckpointedAt
	pod := spec.Annotations["io.kubernetes.pod.name"]
	container := spec.Annotations["io.kubernetes.container.name"]
	containerManager := spec.Annotations["io.container.manager"]

	return &ChkptConfig{
		Namespace:        namespace,
		Pod:              pod,
		Container:        container,
		ContainerManager: containerManager,
		Timestamp:        timestamp,
	}, nil
}
