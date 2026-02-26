package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/checkpoint-restore/checkpointctl/internal"
	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/spf13/cobra"
)

func Diff() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <checkpointA> <checkpointB>",
		Short: "Show changes between two container checkpoints",
		Long: `Compare two CRIU checkpoints and show differences in:
  - Process tree (new/removed/modified processes)
  - File descriptors (opened/closed files)
  - Memory usage (size changes)

Example:
  checkpointctl diff checkpoint1.tar checkpoint2.tar
  checkpointctl diff --format json checkpoint1.tar checkpoint2.tar
  checkpointctl diff --files --ps-tree-cmd checkpoint1.tar checkpoint2.tar`,
		Args: cobra.ExactArgs(2),
		RunE: diff,
	}

	flags := cmd.Flags()
	flags.StringVar(
		format,
		"format",
		"tree",
		"Specify output format: tree or json",
	)
	flags.BoolVar(
		psTreeCmd,
		"ps-tree-cmd",
		false,
		"Include full command lines in process tree diff",
	)
	flags.BoolVar(
		psTreeEnv,
		"ps-tree-env",
		false,
		"Include environment variables in process tree diff",
	)
	flags.BoolVar(
		files,
		"files",
		false,
		"Include file descriptors in the diff",
	)
	flags.BoolVar(
		sockets,
		"sockets",
		false,
		"Include sockets in the diff",
	)

	return cmd
}

func diff(cmd *cobra.Command, args []string) error {
	checkpointA := args[0]
	checkpointB := args[1]

	// Build required files list based on flags (same as inspect)
	requiredFiles := []string{
		metadata.SpecDumpFile,
		metadata.ConfigDumpFile,
	}

	// Add process tree files
	requiredFiles = append(
		requiredFiles,
		filepath.Join(metadata.CheckpointDirectory, "pstree.img"),
		filepath.Join(metadata.CheckpointDirectory, "core-"),
	)

	if *psTreeCmd || *psTreeEnv {
		requiredFiles = append(
			requiredFiles,
			filepath.Join(metadata.CheckpointDirectory, "pagemap-"),
			filepath.Join(metadata.CheckpointDirectory, "pages-"),
			filepath.Join(metadata.CheckpointDirectory, "mm-"),
		)
	}

	if *files {
		requiredFiles = append(
			requiredFiles,
			filepath.Join(metadata.CheckpointDirectory, "files.img"),
			filepath.Join(metadata.CheckpointDirectory, "fs-"),
			filepath.Join(metadata.CheckpointDirectory, "ids-"),
			filepath.Join(metadata.CheckpointDirectory, "fdinfo-"),
		)
	}

	if *sockets {
		requiredFiles = append(
			requiredFiles,
			filepath.Join(metadata.CheckpointDirectory, "files.img"),
			filepath.Join(metadata.CheckpointDirectory, "ids-"),
			filepath.Join(metadata.CheckpointDirectory, "fdinfo-"),
		)
	}

	// Load checkpoint A
	tasksA, err := internal.CreateTasks([]string{checkpointA}, requiredFiles)
	if err != nil {
		return fmt.Errorf("failed to load checkpointA: %w", err)
	}
	defer internal.CleanupTasks(tasksA)

	// Load checkpoint B
	tasksB, err := internal.CreateTasks([]string{checkpointB}, requiredFiles)
	if err != nil {
		return fmt.Errorf("failed to load checkpointB: %w", err)
	}
	defer internal.CleanupTasks(tasksB)

	if len(tasksA) == 0 || len(tasksB) == 0 {
		return fmt.Errorf("failed to load checkpoint data")
	}

	// Get JSON representation of both checkpoints
	jsonA, err := getTaskJSON(tasksA)
	if err != nil {
		return fmt.Errorf("failed to serialize checkpointA: %w", err)
	}

	jsonB, err := getTaskJSON(tasksB)
	if err != nil {
		return fmt.Errorf("failed to serialize checkpointB: %w", err)
	}

	// Verify same container
	if jsonA[0].ID != jsonB[0].ID {
		return fmt.Errorf(
			"checkpoints are from different containers:\n  A: %s (%s)\n  B: %s (%s)",
			jsonA[0].ContainerName,
			jsonA[0].ID,
			jsonB[0].ContainerName,
			jsonB[0].ID,
		)
	}

	// Compute diff
	result := computeDiff(jsonA[0], jsonB[0])

	// Render output
	switch *format {
	case "tree":
		renderTreeDiff(result)
		return nil
	case "json":
		return renderJSONDiff(result)
	default:
		return fmt.Errorf("invalid output format: %s", *format)
	}
}

// getTaskJSON converts tasks to JSON format (same as internal.RenderJSONView)
func getTaskJSON(tasks []internal.Task) ([]CheckpointMetadata, error) {
	var result []CheckpointMetadata

	for _, task := range tasks {
		// Use the same rendering logic as inspect
		var buf []byte
		var err error

		// Create a pipe to capture RenderJSONView output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Call the existing RenderJSONView
		err = internal.RenderJSONView([]internal.Task{task})

		w.Close()
		os.Stdout = oldStdout

		if err != nil {
			return nil, err
		}

		// Read captured output
		buf = make([]byte, 1024*1024) // 1MB buffer
		n, _ := r.Read(buf)

		// Parse JSON
		var metadata []CheckpointMetadata
		if err := json.Unmarshal(buf[:n], &metadata); err != nil {
			return nil, err
		}

		if len(metadata) > 0 {
			result = append(result, metadata[0])
		}
	}

	return result, nil
}

// Structs matching JSON output from inspect
type CheckpointMetadata struct {
	ContainerName   string                `json:"container_name"`
	Image           string                `json:"image"`
	ID              string                `json:"id"`
	Runtime         string                `json:"runtime"`
	Created         string                `json:"created"`
	Engine          string                `json:"engine"`
	CheckpointSize  CheckpointSize        `json:"checkpoint_size"`
	ProcessTree     *ProcessTree          `json:"process_tree,omitempty"`
	FileDescriptors []FileDescriptorEntry `json:"file_descriptors,omitempty"`
}

type CheckpointSize struct {
	TotalSize       int64 `json:"total_size"`
	MemoryPagesSize int64 `json:"memory_pages_size"`
}

type ProcessTree struct {
	PID      int            `json:"pid"`
	Command  string         `json:"command"`
	Cmdline  string         `json:"cmdline"`
	Children []*ProcessTree `json:"children,omitempty"`
}

type FileDescriptorEntry struct {
	PID       int        `json:"pid"`
	OpenFiles []OpenFile `json:"open_files"`
}

type OpenFile struct {
	Type string `json:"type"`
	FD   string `json:"fd"`
	Path string `json:"path"`
}

// Diff result structures
type DiffResult struct {
	ContainerID    string         `json:"container_id"`
	ContainerName  string         `json:"container_name"`
	Image          string         `json:"image"`
	CheckpointA    CheckpointInfo `json:"checkpoint_a"`
	CheckpointB    CheckpointInfo `json:"checkpoint_b"`
	ProcessChanges *ProcessDiff   `json:"process_changes,omitempty"`
	FileChanges    *FileDiff      `json:"file_changes,omitempty"`
	MemoryChanges  *MemoryDiff    `json:"memory_changes"`
	Summary        string         `json:"summary"`
}

type CheckpointInfo struct {
	Created   string `json:"created"`
	TotalSize int64  `json:"total_size"`
}

type ProcessDiff struct {
	Added     []ProcessInfo `json:"added,omitempty"`
	Removed   []ProcessInfo `json:"removed,omitempty"`
	Modified  []ProcessInfo `json:"modified,omitempty"`
	Unchanged int           `json:"unchanged"`
}

type ProcessInfo struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	Cmdline string `json:"cmdline,omitempty"`
}

type FileDiff struct {
	Added     []FileInfo `json:"added,omitempty"`
	Removed   []FileInfo `json:"removed,omitempty"`
	Unchanged int        `json:"unchanged"`
}

type FileInfo struct {
	Path string `json:"path"`
	Type string `json:"type"`
	PID  int    `json:"pid"`
	FD   string `json:"fd"`
}

type MemoryDiff struct {
	SizeChangeBytes int64   `json:"size_change_bytes"`
	SizeChangeMB    float64 `json:"size_change_mb"`
}

func computeDiff(metadataA, metadataB CheckpointMetadata) *DiffResult {
	result := &DiffResult{
		ContainerID:   metadataA.ID,
		ContainerName: metadataA.ContainerName,
		Image:         metadataA.Image,
		CheckpointA: CheckpointInfo{
			Created:   metadataA.Created,
			TotalSize: metadataA.CheckpointSize.TotalSize,
		},
		CheckpointB: CheckpointInfo{
			Created:   metadataB.Created,
			TotalSize: metadataB.CheckpointSize.TotalSize,
		},
	}

	// Compare processes
	result.ProcessChanges = compareProcessTrees(metadataA.ProcessTree, metadataB.ProcessTree)

	// Compare files if requested
	if *files {
		result.FileChanges = compareFileDescriptors(metadataA.FileDescriptors, metadataB.FileDescriptors)
	}

	// Compare memory
	sizeChange := metadataB.CheckpointSize.MemoryPagesSize - metadataA.CheckpointSize.MemoryPagesSize
	result.MemoryChanges = &MemoryDiff{
		SizeChangeBytes: sizeChange,
		SizeChangeMB:    float64(sizeChange) / 1024 / 1024,
	}

	// Generate summary
	result.Summary = generateSummary(result)

	return result
}

func compareProcessTrees(treeA, treeB *ProcessTree) *ProcessDiff {
	diff := &ProcessDiff{}

	// Flatten both trees
	procsA := flattenProcessTree(treeA)
	procsB := flattenProcessTree(treeB)

	// Build PID maps
	mapA := make(map[int]ProcessInfo)
	mapB := make(map[int]ProcessInfo)

	for _, p := range procsA {
		mapA[p.PID] = p
	}
	for _, p := range procsB {
		mapB[p.PID] = p
	}

	// Find differences
	for pid, procB := range mapB {
		if procA, exists := mapA[pid]; !exists {
			diff.Added = append(diff.Added, procB)
		} else if *psTreeCmd && procA.Cmdline != procB.Cmdline {
			diff.Modified = append(diff.Modified, procB)
		} else {
			diff.Unchanged++
		}
	}

	for pid := range mapA {
		if _, exists := mapB[pid]; !exists {
			diff.Removed = append(diff.Removed, mapA[pid])
		}
	}

	return diff
}

func flattenProcessTree(tree *ProcessTree) []ProcessInfo {
	if tree == nil {
		return nil
	}

	var processes []ProcessInfo

	var flatten func(*ProcessTree)
	flatten = func(node *ProcessTree) {
		if node == nil {
			return
		}

		proc := ProcessInfo{
			PID:     node.PID,
			Command: node.Command,
		}
		if *psTreeCmd {
			proc.Cmdline = node.Cmdline
		}
		processes = append(processes, proc)

		for _, child := range node.Children {
			flatten(child)
		}
	}

	flatten(tree)
	return processes
}

func compareFileDescriptors(fdsA, fdsB []FileDescriptorEntry) *FileDiff {
	diff := &FileDiff{}

	// Build file maps
	mapA := make(map[string]FileInfo)
	mapB := make(map[string]FileInfo)

	for _, entry := range fdsA {
		for _, file := range entry.OpenFiles {
			key := fmt.Sprintf("%d:%s:%s", entry.PID, file.FD, file.Path)
			mapA[key] = FileInfo{
				PID:  entry.PID,
				Type: file.Type,
				FD:   file.FD,
				Path: file.Path,
			}
		}
	}

	for _, entry := range fdsB {
		for _, file := range entry.OpenFiles {
			key := fmt.Sprintf("%d:%s:%s", entry.PID, file.FD, file.Path)
			mapB[key] = FileInfo{
				PID:  entry.PID,
				Type: file.Type,
				FD:   file.FD,
				Path: file.Path,
			}
		}
	}

	// Find differences
	for key, fileB := range mapB {
		if _, exists := mapA[key]; !exists {
			diff.Added = append(diff.Added, fileB)
		} else {
			diff.Unchanged++
		}
	}

	for key, fileA := range mapA {
		if _, exists := mapB[key]; !exists {
			diff.Removed = append(diff.Removed, fileA)
		}
	}

	return diff
}

func generateSummary(result *DiffResult) string {
	summary := fmt.Sprintf("Checkpoint comparison for container %s", result.ContainerName)

	if result.ProcessChanges != nil {
		added := len(result.ProcessChanges.Added)
		removed := len(result.ProcessChanges.Removed)
		modified := len(result.ProcessChanges.Modified)

		if added > 0 || removed > 0 || modified > 0 {
			summary += fmt.Sprintf("\nProcesses: +%d -%d ~%d", added, removed, modified)
		}
	}

	if result.FileChanges != nil {
		added := len(result.FileChanges.Added)
		removed := len(result.FileChanges.Removed)

		if added > 0 || removed > 0 {
			summary += fmt.Sprintf("\nFiles: +%d -%d", added, removed)
		}
	}

	if result.MemoryChanges != nil && result.MemoryChanges.SizeChangeBytes != 0 {
		summary += fmt.Sprintf("\nMemory: %+.2f MB", result.MemoryChanges.SizeChangeMB)
	}

	return summary
}

func renderTreeDiff(result *DiffResult) {
	fmt.Printf("╔════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ Checkpoint Diff                                                ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Container: %-51s ║\n", truncate(result.ContainerName, 51))
	fmt.Printf("║ Image:     %-51s ║\n", truncate(result.Image, 51))
	fmt.Printf("║ ID:        %-51s ║\n", truncate(result.ContainerID, 51))
	fmt.Printf("╚════════════════════════════════════════════════════════════════╝\n\n")

	// Checkpoint info
	fmt.Printf("Checkpoint A:\n")
	fmt.Printf("  Created: %s\n", result.CheckpointA.Created)
	fmt.Printf("  Size:    %d bytes\n\n", result.CheckpointA.TotalSize)

	fmt.Printf("Checkpoint B:\n")
	fmt.Printf("  Created: %s\n", result.CheckpointB.Created)
	fmt.Printf("  Size:    %d bytes\n\n", result.CheckpointB.TotalSize)

	// Memory changes
	if result.MemoryChanges != nil {
		fmt.Println("┌─ Memory Changes ─────────────────────────────────────────────┐")
		if result.MemoryChanges.SizeChangeBytes > 0 {
			fmt.Printf("│ ↑ Increased by %.2f MB\n", result.MemoryChanges.SizeChangeMB)
		} else if result.MemoryChanges.SizeChangeBytes < 0 {
			fmt.Printf("│ ↓ Decreased by %.2f MB\n", -result.MemoryChanges.SizeChangeMB)
		} else {
			fmt.Println("│ = No change")
		}
		fmt.Println("└──────────────────────────────────────────────────────────────┘")
	}

	// Process changes
	if result.ProcessChanges != nil {
		fmt.Println("┌─ Process Changes ────────────────────────────────────────────┐")

		if len(result.ProcessChanges.Added) > 0 {
			fmt.Println("│ Added:")
			for _, proc := range result.ProcessChanges.Added {
				fmt.Printf("│   + PID %-5d %s\n", proc.PID, proc.Command)
				if *psTreeCmd && proc.Cmdline != "" {
					fmt.Printf("│             %s\n", truncate(proc.Cmdline, 55))
				}
			}
		}

		if len(result.ProcessChanges.Removed) > 0 {
			fmt.Println("│ Removed:")
			for _, proc := range result.ProcessChanges.Removed {
				fmt.Printf("│   - PID %-5d %s\n", proc.PID, proc.Command)
			}
		}

		if len(result.ProcessChanges.Modified) > 0 {
			fmt.Println("│ Modified:")
			for _, proc := range result.ProcessChanges.Modified {
				fmt.Printf("│   ~ PID %-5d %s\n", proc.PID, proc.Command)
			}
		}

		if result.ProcessChanges.Unchanged > 0 {
			fmt.Printf("│ Unchanged: %d\n", result.ProcessChanges.Unchanged)
		}
		fmt.Println("└──────────────────────────────────────────────────────────────┘")
	}

	// File changes
	if result.FileChanges != nil {
		fmt.Println("┌─ File Descriptor Changes ────────────────────────────────────┐")

		if len(result.FileChanges.Added) > 0 {
			fmt.Println("│ Added:")
			for _, file := range result.FileChanges.Added {
				fmt.Printf("│   + PID %-5d %-8s %s\n",
					file.PID, file.Type, truncate(file.Path, 38))
			}
		}

		if len(result.FileChanges.Removed) > 0 {
			fmt.Println("│ Removed:")
			for _, file := range result.FileChanges.Removed {
				fmt.Printf("│   - PID %-5d %-8s %s\n",
					file.PID, file.Type, truncate(file.Path, 38))
			}
		}

		if result.FileChanges.Unchanged > 0 {
			fmt.Printf("│ Unchanged: %d\n", result.FileChanges.Unchanged)
		}
		fmt.Println("└──────────────────────────────────────────────────────────────┘")
	}

	// Summary
	fmt.Println("Summary:")
	fmt.Println(result.Summary)
}

func renderJSONDiff(result *DiffResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
