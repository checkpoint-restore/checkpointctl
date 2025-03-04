// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/checkpoint-restore/checkpointctl/internal"
	"github.com/spf13/cobra"
)

func BuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build <checkpoint-path> <image-name>",
		Short: "Create an OCI image from a container checkpoint archive",
		Long: `The 'build' command converts a container checkpoint archive into an OCI-compatible image.
Metadata from the checkpoint archive is extracted and applied as OCI image annotations.
Example:
  checkpointctl build checkpoint.tar quay.io/foo/bar:latest
  buildah push quay.io/foo/bar:latest`,
		Args: cobra.ExactArgs(2),
		RunE: convertArchive,
	}

	return cmd
}

func convertArchive(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("please provide both the checkpoint path and the image name")
	}

	checkpointPath := args[0]
	imageName := args[1]

	ImageBuilder := internal.NewImageBuilder(imageName, checkpointPath)

	err := ImageBuilder.CreateImageFromCheckpoint(context.Background())
	if err != nil {
		return err
	}

	log.Printf("Image '%s' created successfully from checkpoint '%s'\n", imageName, checkpointPath)
	return nil
}
