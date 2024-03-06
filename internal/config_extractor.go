package internal

import (
	"log"
	"os"
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

	info := &checkpointInfo{}
	info.configDump, _, err = metadata.ReadContainerCheckpointConfigDump(tempDir)
	if err != nil {
		return nil, err
	}
	info.specDump, _, err = metadata.ReadContainerCheckpointSpecDump(tempDir)
	if err != nil {
		return nil, err
	}

	info.containerInfo, err = getContainerInfo(info.specDump, info.configDump)
	if err != nil {
		return nil, err
	}
	return &ChkptConfig{
		Namespace:        info.containerInfo.Namespace,
		Pod:              info.containerInfo.Pod,
		Container:        info.containerInfo.Name,
		ContainerManager: info.containerInfo.Engine,
		Timestamp:        info.configDump.CheckpointedAt,
	}, nil
}
