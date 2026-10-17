// SPDX-License-Identifier: Apache-2.0

// Command lib-size-test reads checkpoint files for the library size check.
package main

import (
	"fmt"
	"os"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: lib-size-test CHECKPOINT_DIRECTORY")
		os.Exit(1)
	}
	config, _, err := metadata.ReadContainerCheckpointConfigDump(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	spec, _, err := metadata.ReadContainerCheckpointSpecDump(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Container: %s (%s), OCI version: %s\n", config.Name, config.ID, spec.Version)
}
