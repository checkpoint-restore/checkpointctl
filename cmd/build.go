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
		Use:   "build [checkpoint-path] [image-name]",
		Short: "Create an OCI image from a container checkpoint archive",
		RunE:  convertArchive,
	}

	return cmd
}

func convertArchive(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("please provide both the checkpoint path and the image name")
	}

	checkpointPath := args[0]
	imageName := args[1]

	imageCreater := internal.NewImageCreator(imageName, checkpointPath)

	err := imageCreater.CreateImageFromCheckpoint(context.Background())
	if err != nil {
		return err
	}

	log.Printf("Image '%s' created successfully from checkpoint '%s'\n", imageName, checkpointPath)
	return nil
}
