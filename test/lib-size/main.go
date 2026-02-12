// SPDX-License-Identifier: Apache-2.0

// This is a minimal program that imports the lib/metadata package
// to measure the size impact of including the library in other projects.
package main

import (
	metadata "github.com/checkpoint-restore/checkpointctl/lib"
)

func main() {
	// Use exported symbols to ensure they are not optimized away
	_ = metadata.ConfigDumpFile
	_ = metadata.CheckpointAnnotationEngine
}
