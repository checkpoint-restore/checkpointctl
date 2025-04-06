package internal

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
)

type ImageBuilder struct {
	imageName      string
	checkpointPath string
}

func NewImageBuilder(imageName, checkpointPath string) *ImageBuilder {
	return &ImageBuilder{
		imageName:      imageName,
		checkpointPath: checkpointPath,
	}
}

func runBuildahCommand(args ...string) (string, error) {
	cmd := exec.Command("buildah", args...)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("buildah command failed: error: %w, stderr: %s", err, stderr.String())
	}

	return out.String(), nil
}

func (ic *ImageBuilder) CreateImageFromCheckpoint(ctx context.Context) error {
	// Step 1: Create a new container from scratch
	newContainer, err := runBuildahCommand("from", "scratch")
	if err != nil {
		return fmt.Errorf("creating container failed: %w", err)
	}
	newContainer = strings.TrimSpace(newContainer)

	// Ensure the container is removed in case of failure
	defer func() {
		if newContainer != "" {
			_, err := runBuildahCommand("rm", newContainer)
			if err != nil {
				fmt.Printf("Warning: failed to remove container %s: %v\n", newContainer, err)
			}
		}
	}()

	// Step 2: Add checkpoint files to the container
	_, err = runBuildahCommand("add", newContainer, ic.checkpointPath)
	if err != nil {
		return fmt.Errorf("adding files to container failed: %w", err)
	}

	// Step 3: Apply checkpoint annotations
	checkpointImageAnnotations, err := ic.getCheckpointAnnotations()
	if err != nil {
		return fmt.Errorf("extracting checkpoint annotations failed: %w", err)
	}

	for key, value := range checkpointImageAnnotations {
		_, err := runBuildahCommand("config", "--annotation", fmt.Sprintf("%s=%s", key, value), newContainer)
		if err != nil {
			fmt.Printf("Error setting annotation %s=%s: %v\n", key, value, err)
		} else {
			fmt.Printf("Added annotation: %s=%s\n", key, value)
		}
	}

	// Step 4: Commit the container to an image
	_, err = runBuildahCommand("commit", newContainer, ic.imageName)
	if err != nil {
		return fmt.Errorf("committing container annotations failed: %w", err)
	}

	return nil
}

func (ic *ImageBuilder) getCheckpointAnnotations() (map[string]string, error) {
	checkpointImageAnnotations := map[string]string{}

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "checkpoint-extract-")
	if err != nil {
		log.Printf("Error creating temporary directory: %v\n", err)
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	filesToExtract := []string{"spec.dump", "config.dump"}
	if err = UntarFiles(ic.checkpointPath, tempDir, filesToExtract); err != nil {
		log.Printf("Error extracting files from archive %s: %v\n", ic.checkpointPath, err)
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

	task := Task{
		OutputDir:          tempDir,
		CheckpointFilePath: ic.checkpointPath,
	}
	info.containerInfo, err = getContainerInfo(info.specDump, info.configDump, task)
	if err != nil {
		return nil, err
	}

	checkpointImageAnnotations[metadata.CheckpointAnnotationEngine] = info.containerInfo.Engine
	checkpointImageAnnotations[metadata.CheckpointAnnotationName] = info.containerInfo.Name
	checkpointImageAnnotations[metadata.CheckpointAnnotationPod] = info.containerInfo.Pod
	checkpointImageAnnotations[metadata.CheckpointAnnotationNamespace] = info.containerInfo.Namespace
	checkpointImageAnnotations[metadata.CheckpointAnnotationRootfsImageUserRequested] = info.configDump.RootfsImage
	checkpointImageAnnotations[metadata.CheckpointAnnotationRootfsImageName] = info.configDump.RootfsImageName
	checkpointImageAnnotations[metadata.CheckpointAnnotationRootfsImageID] = info.configDump.RootfsImageRef
	checkpointImageAnnotations[metadata.CheckpointAnnotationRuntimeName] = info.configDump.OCIRuntime

	return checkpointImageAnnotations, nil
}
