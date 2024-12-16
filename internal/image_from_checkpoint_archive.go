package internal

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
)

const (
	BUILD_SCRIPT  = "/usr/libexec/build_image.sh"
	PODMAN_ENGINE = "libpod"
)

type ImageCreator struct {
	imageName      string
	checkpointPath string
}

func NewImageCreator(imageName, checkpointPath string) *ImageCreator {
	return &ImageCreator{
		imageName:      imageName,
		checkpointPath: checkpointPath,
	}
}

func (ic *ImageCreator) CreateImageFromCheckpoint(ctx context.Context) error {
	tempDir, err := os.MkdirTemp("", "checkpoint_tmp")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	annotationsFilePath, err := ic.setCheckpointAnnotations(tempDir)
	if err != nil {
		return err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command(BUILD_SCRIPT, "-a", annotationsFilePath, "-c", ic.checkpointPath, "-i", ic.imageName)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to execute script: %v, %v, %w", stdout.String(), stderr.String(), err)
	}

	return nil
}

func writeAnnotationsToFile(tempDir string, annotations map[string]string) (string, error) {
	tempFile, err := os.CreateTemp(tempDir, "annotations_*.txt")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	for key, value := range annotations {
		_, err := fmt.Fprintf(tempFile, "%s=%s\n", key, value)
		if err != nil {
			return "", err
		}
	}

	return tempFile.Name(), nil
}

func (ic *ImageCreator) setCheckpointAnnotations(tempDir string) (string, error) {
	filesToExtract := []string{"spec.dump", "config.dump"}
	if err := UntarFiles(ic.checkpointPath, tempDir, filesToExtract); err != nil {
		log.Printf("Error extracting files from archive %s: %v\n", ic.checkpointPath, err)
		return "", err
	}

	var err error
	info := &checkpointInfo{}
	info.configDump, _, err = metadata.ReadContainerCheckpointConfigDump(tempDir)
	if err != nil {
		return "", err
	}

	info.specDump, _, err = metadata.ReadContainerCheckpointSpecDump(tempDir)
	if err != nil {
		return "", err
	}

	info.containerInfo, err = getContainerInfo(info.specDump, info.configDump)
	if err != nil {
		return "", err
	}

	checkpointImageAnnotations := map[string]string{}
	checkpointImageAnnotations[metadata.CheckpointAnnotationEngine] = info.containerInfo.Engine
	checkpointImageAnnotations[metadata.CheckpointAnnotationName] = info.containerInfo.Name
	checkpointImageAnnotations[metadata.CheckpointAnnotationPod] = info.containerInfo.Pod
	checkpointImageAnnotations[metadata.CheckpointAnnotationNamespace] = info.containerInfo.Namespace
	checkpointImageAnnotations[metadata.CheckpointAnnotationRootfsImageUserRequested] = info.configDump.RootfsImage
	checkpointImageAnnotations[metadata.CheckpointAnnotationRootfsImageName] = info.configDump.RootfsImageName
	checkpointImageAnnotations[metadata.CheckpointAnnotationRootfsImageID] = info.configDump.RootfsImageRef
	checkpointImageAnnotations[metadata.CheckpointAnnotationRuntimeName] = info.configDump.OCIRuntime

	annotationsFilePath, err := writeAnnotationsToFile(tempDir, checkpointImageAnnotations)
	if err != nil {
		return "", err
	}

	return annotationsFilePath, nil
}
