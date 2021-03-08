// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/containers/storage/pkg/archive"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	kubeletCheckpointsDirectory = "/var/lib/kubelet/checkpoints"
	name                        string
	showPodUID                  bool
	showPodIP                   bool
	output                      string
	input                       string
	compress                    string
)

func main() {
	rootCommand := &cobra.Command{
		Use:   name,
		Short: name + " is a tool to read and manipulate checkpoint archives",
		Long: name + " is a tool to read and manipulate checkpoint archives as " +
			"created by Podman, CRI-O and Kubernetes",
		SilenceUsage: true,
	}

	showCommand := setupShow()
	rootCommand.AddCommand(showCommand)

	extractCommand := setupExtract()
	rootCommand.AddCommand(extractCommand)

	insertCommand := setupInsert()
	rootCommand.AddCommand(insertCommand)

	if err := rootCommand.Execute(); err != nil {
		os.Exit(1)
	}
}

func setupShow() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show information about available checkpoints",
		RunE:  show,
	}
	flags := cmd.Flags()
	flags.StringVarP(
		&kubeletCheckpointsDirectory,
		"target", "t",
		kubeletCheckpointsDirectory,
		"Directory or archive which contains the Kubernetes checkpoints and metadata",
	)
	flags.BoolVar(
		&showPodUID,
		"show-pod-uid",
		false,
		"Show Pod UID in output",
	)
	flags.BoolVar(
		&showPodIP,
		"show-pod-ip",
		false,
		"Show Pod IP in output",
	)

	return cmd
}

func show(cmd *cobra.Command, args []string) error {
	checkpointDirectory := kubeletCheckpointsDirectory

	tar, err := os.Stat(kubeletCheckpointsDirectory)
	if err != nil {
		return errors.Wrapf(err, "Target %s access error\n", kubeletCheckpointsDirectory)
	}

	if tar.Mode().IsRegular() {
		dir, err := ioutil.TempDir("", "kubelet-checkpoint")
		if err != nil {
			return errors.Wrapf(err, "Creating temporary directory failed\n")
		}

		defer func() {
			if err := os.RemoveAll(dir); err != nil {
				fmt.Fprintf(os.Stderr, "Could not recursively remove %s: %q", dir, err)
			}
		}()

		if err := archive.UntarPath(kubeletCheckpointsDirectory, dir); err != nil {
			return errors.Wrapf(err, "Unpacking of checkpoint archive %s failed\n", kubeletCheckpointsDirectory)
		}
		checkpointDirectory = dir
	}

	archiveType, err := metadata.DetectCheckpointArchiveType(checkpointDirectory)
	if err != nil {
		return err
	}

	switch archiveType {
	case metadata.Kubelet:
		return showKubeletCheckpoint(checkpointDirectory)
	case metadata.Container:
		return showContainerCheckpoint(checkpointDirectory)
	case metadata.Pod:
		return showPodCheckpoint(checkpointDirectory)
	case metadata.Unknown:
		return errors.Errorf("%q contains unknown archive type\n", kubeletCheckpointsDirectory)
	}

	return nil
}

func showKubeletCheckpoint(checkpointDirectory string) error {
	checkpointMetadata, checkpointMetadataPath, err := metadata.ReadKubeletCheckpoints(checkpointDirectory)
	if err != nil {
		return errors.Wrapf(err, "Reading %q failed\n", checkpointMetadataPath)
	}

	fmt.Printf("\nDisplaying kubelet checkpoint data from %s\n\n", checkpointMetadataPath)

	table := tablewriter.NewWriter(os.Stdout)
	header := []string{
		"Pod",
		"Namespace",
		"Container",
		"Image",
		"Archive Found",
	}

	if showPodUID {
		header = append(header, "Pod UID")
	}
	if showPodIP {
		header = append(header, "Pod IP")
	}
	table.SetHeader(header)
	table.SetAutoMergeCells(true)
	table.SetRowLine(true)
	for _, p := range checkpointMetadata.CheckpointedPods {
		var row []string
		var exists bool

		archive := filepath.Join(checkpointDirectory, p.ID+".tar")
		if _, err := os.Stat(archive); err != nil {
			exists = false
		} else {
			exists = true
		}

		for _, c := range p.Containers {
			row = []string{p.Name, p.Namespace, c.Name, c.Image, strconv.FormatBool(exists)}
			if showPodUID {
				row = append(row, p.PodUID)
			}
			if showPodIP {
				row = append(row, p.PodIP)
			}
			table.Append(row)
		}
	}

	table.Render()

	return nil
}

func validateExtract(c *cobra.Command, args []string) error {
	value, _ := c.Flags().GetString("output")
	if value == "" {
		if err := c.Help(); err != nil {
			return errors.Wrapf(err, "Printing help failed")
		}
		fmt.Println()

		return errors.Errorf("Specifying an output file (-o|--output) is required\n")
	}

	return nil
}

func setupExtract() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Extract all kubelet checkpoints",
		RunE:  extract,
		Args:  validateExtract,
	}
	flags := cmd.Flags()
	flags.StringVarP(
		&kubeletCheckpointsDirectory,
		"target", "t",
		kubeletCheckpointsDirectory,
		"Directory which contains the Kubernetes checkpoints and metadata",
	)
	flags.StringVarP(
		&output,
		"output", "o",
		"",
		"Destination for the tar archive containing the extracted checkpointed pods",
	)
	flags.StringVarP(
		&compress,
		"compress", "c",
		"zstd",
		"Select compression algorithm (gzip, none, zstd)",
	)

	return cmd
}

func extract(cmd *cobra.Command, args []string) error {
	var compression archive.Compression

	switch strings.ToLower(compress) {
	case "none":
		compression = archive.Uncompressed
	case "gzip":
		compression = archive.Gzip
	case "zstd":
		compression = archive.Zstd
	default:
		return errors.Errorf("Select compression algorithm (%q) not supported\n", compress)
	}
	checkpointMetadata, checkpointMetadataPath, err := metadata.ReadKubeletCheckpoints(kubeletCheckpointsDirectory)
	if err != nil {
		return errors.Wrapf(err, "Reading %q failed\n", checkpointMetadataPath)
	}

	fmt.Printf("\nExtracting checkpoint data from %s\n\n", checkpointMetadataPath)

	includeFiles := []string{
		metadata.CheckpointedPodsFile,
	}

	for _, p := range checkpointMetadata.CheckpointedPods {
		checkpointArchive := filepath.Join(kubeletCheckpointsDirectory, p.ID+".tar")
		if _, err := os.Stat(checkpointArchive); err != nil {
			return errors.Wrapf(err, "Cannot access %q failed\n", checkpointArchive)
		}

		includeFiles = append(includeFiles, p.ID+".tar")
	}

	input, err := archive.TarWithOptions(kubeletCheckpointsDirectory, &archive.TarOptions{
		Compression:  compression,
		IncludeFiles: includeFiles,
	})
	if err != nil {
		return errors.Wrapf(err, "Cannot create tar archive %q failed\n", output)
	}

	outFile, err := os.Create(output)
	if err != nil {
		return errors.Wrapf(err, "Cannot create tar archive %q failed\n", output)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, input)
	if err != nil {
		return errors.Wrapf(err, "Cannot create tar archive %q failed\n", output)
	}

	return nil
}

func setupInsert() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "insert",
		Short: "Insert an extracted kubelet checkpoint into a kubelet checkpoints directory",
		RunE:  insert,
	}
	flags := cmd.Flags()
	flags.StringVarP(
		&kubeletCheckpointsDirectory,
		"target", "t",
		kubeletCheckpointsDirectory,
		"Directory which contains the Kubernetes checkpoints and metadata",
	)
	flags.StringVarP(
		&input,
		"input", "i",
		"",
		"Location of the tar archive containing the extracted checkpointed pods",
	)

	return cmd
}

func insert(cmd *cobra.Command, args []string) error {
	dir, err := ioutil.TempDir("", "kubelet-checkpoint")
	if err != nil {
		return errors.Wrapf(err, "Creating temporary directory failed\n")
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintf(os.Stderr, "Could not recursively remove %s: %q", dir, err)
		}
	}()

	// Although there is TarOptions.ExcludePatterns to exclude files it is not
	// possible to exclude <randomstring>.tar files as the ExcludePattern is used
	// with strings.HasPrefix(), which does not help if we only know that the
	// files we would like to exclude ends with ".tar".

	// At this point we are extracting therefore everything in a temporary location
	// and moving it later to the kubeletCheckpointsDirectory.

	if err := archive.UntarPath(input, dir); err != nil {
		return errors.Wrapf(err, "Unpacking of checkpoint archive %s failed\n", input)
	}

	insertData, _, err := metadata.ReadKubeletCheckpoints(dir)
	if err != nil {
		return errors.Wrapf(err, "%s not a kubelet checkpoint archive\n", input)
	}

	// Remove the pod checkpoints immediately
	for _, p := range insertData.CheckpointedPods {
		archive := filepath.Join(dir, p.ID+".tar")
		if err := os.Remove(archive); err != nil {
			return errors.Wrapf(err, "Unable to delete %q. This should not happen", archive)
		}
	}

	kubeletData, _, err := metadata.ReadKubeletCheckpoints(kubeletCheckpointsDirectory)
	if os.IsNotExist(errors.Unwrap(errors.Unwrap(err))) {
		// There is no existing checkpointed.pods file
		kubeletData = insertData
	} else {
		if err != nil {
			return errors.Wrapf(err, "could not read kubelet checkpoints metadata at %s\n", kubeletCheckpointsDirectory)
		}

		kubeletData.CheckpointedPods = append(kubeletData.CheckpointedPods, insertData.CheckpointedPods...)
	}

	// Now untar the checkpointed pods to the destination directory
	options := &archive.TarOptions{
		// Exclude the metadata
		ExcludePatterns: []string{
			metadata.CheckpointedPodsFile,
		},
	}
	archiveFile, err := os.Open(input)
	if err != nil {
		return errors.Wrapf(err, "Failed to open checkpointed pods archive %s for import", input)
	}

	err = archive.Untar(archiveFile, kubeletCheckpointsDirectory, options)
	if err != nil {
		return errors.Wrapf(err, "Unpacking of checkpointed pods archive %s failed", input)
	}

	if err := metadata.WriteKubeletCheckpointsMetadata(kubeletData, kubeletCheckpointsDirectory); err != nil {
		return err
	}

	return nil
}
