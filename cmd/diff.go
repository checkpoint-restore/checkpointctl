package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/checkpoint-restore/checkpointctl/internal"
	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/spf13/cobra"
	"github.com/xlab/treeprint"
)

func Diff() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <checkpointA> <checkpointB>",
		Short: "Show changes between two container checkpoints",
		Long: `Compare two CRIU checkpoints and show differences in:
  - Process tree (new/removed/modified processes)
  - File descriptors (opened/closed files)
  - Sockets (new/removed network sockets)
  - Memory usage (size changes)

Example:
  checkpointctl diff checkpoint1.tar checkpoint2.tar
  checkpointctl diff --format json checkpoint1.tar checkpoint2.tar
  checkpointctl diff --files --ps-tree-cmd checkpoint1.tar checkpoint2.tar
  checkpointctl diff --files --sockets checkpoint1.tar checkpoint2.tar`,
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
	flags.BoolVar(
		showUnchanged,
		"show-unchanged",
		false,
		"Show entries that are identical between the two checkpoints",
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

	prevPsTree := internal.PsTree
	defer func() { internal.PsTree = prevPsTree }()

	for _, task := range tasks {
		// Enable process-tree extraction only when pstree.img is present; some
		// test fixtures carry just config/spec dumps and have no checkpoint data.
		pstreePath := filepath.Join(task.OutputDir, metadata.CheckpointDirectory, "pstree.img")
		_, statErr := os.Stat(pstreePath)
		internal.PsTree = statErr == nil

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
	ProcessTree     *internal.PsNode      `json:"process_tree,omitempty"`
	FileDescriptors []FileDescriptorEntry `json:"file_descriptors,omitempty"`
	Sockets         []internal.SkNode     `json:"sockets,omitempty"`
}

type CheckpointSize struct {
	TotalSize       int64 `json:"total_size"`
	MemoryPagesSize int64 `json:"memory_pages_size"`
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
	SocketChanges  *SocketDiff    `json:"socket_changes,omitempty"`
	Summary        string         `json:"summary"`

	processTreeB *internal.PsNode
}

type CheckpointInfo struct {
	Created   string `json:"created"`
	TotalSize int64  `json:"total_size"`
}

type ProcessDiff struct {
	Added     []ProcessInfo `json:"added,omitempty"`
	Removed   []ProcessInfo `json:"removed,omitempty"`
	Modified  []ProcessInfo `json:"modified,omitempty"`
	Unchanged []ProcessInfo `json:"unchanged,omitempty"`
}

type ProcessInfo struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	Cmdline string `json:"cmdline,omitempty"`
}

type FileDiff struct {
	Added     []FileInfo `json:"added,omitempty"`
	Removed   []FileInfo `json:"removed,omitempty"`
	Unchanged []FileInfo `json:"unchanged,omitempty"`
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

type SocketDiff struct {
	Added     []SocketInfo `json:"added,omitempty"`
	Removed   []SocketInfo `json:"removed,omitempty"`
	Unchanged []SocketInfo `json:"unchanged,omitempty"`
}

type SocketInfo struct {
	PID      int    `json:"pid"`
	Protocol string `json:"protocol"`
	Type     string `json:"type"`
	Address  string `json:"address,omitempty"`
	State    string `json:"state,omitempty"`
	Source   string `json:"source,omitempty"`
	SrcPort  uint32 `json:"src_port,omitempty"`
	Dest     string `json:"dest,omitempty"`
	DstPort  uint32 `json:"dst_port,omitempty"`
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
	result.processTreeB = metadataB.ProcessTree

	// Compare files if requested
	if *files {
		result.FileChanges = compareFileDescriptors(metadataA.FileDescriptors, metadataB.FileDescriptors)
	}

	// Compare sockets if requested
	if *sockets {
		result.SocketChanges = compareSockets(metadataA.Sockets, metadataB.Sockets)
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

func compareProcessTrees(treeA, treeB *internal.PsNode) *ProcessDiff {
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
			diff.Unchanged = append(diff.Unchanged, procB)
		}
	}

	for pid := range mapA {
		if _, exists := mapB[pid]; !exists {
			diff.Removed = append(diff.Removed, mapA[pid])
		}
	}

	return diff
}

func flattenProcessTree(tree *internal.PsNode) []ProcessInfo {
	if tree == nil {
		return nil
	}

	var processes []ProcessInfo

	var flatten func(*internal.PsNode)
	flatten = func(node *internal.PsNode) {
		if node == nil {
			return
		}

		proc := ProcessInfo{
			PID:     int(node.PID),
			Command: node.Comm,
		}
		if *psTreeCmd {
			proc.Cmdline = node.Cmdline
		}
		processes = append(processes, proc)

		for i := range node.Children {
			flatten(&node.Children[i])
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
			diff.Unchanged = append(diff.Unchanged, fileB)
		}
	}

	for key, fileA := range mapA {
		if _, exists := mapB[key]; !exists {
			diff.Removed = append(diff.Removed, fileA)
		}
	}

	return diff
}

func compareSockets(sksA, sksB []internal.SkNode) *SocketDiff {
	diff := &SocketDiff{}

	// Build socket maps
	// Key format: PID:Protocol:Type:Address:SrcPort:Dest:DstPort
	mapA := make(map[string]SocketInfo)
	mapB := make(map[string]SocketInfo)

	for _, entry := range sksA {
		for _, socket := range entry.OpenSockets {
			key := fmt.Sprintf("%d:%s:%s:%s:%d:%s:%d",
				entry.PID,
				socket.Protocol,
				socket.Data.Type,
				socket.Data.Address,
				socket.Data.SourcePort,
				socket.Data.Dest,
				socket.Data.DestPort,
			)
			mapA[key] = SocketInfo{
				PID:      int(entry.PID),
				Protocol: socket.Protocol,
				Type:     socket.Data.Type,
				Address:  socket.Data.Address,
				State:    socket.Data.State,
				Source:   socket.Data.Source,
				SrcPort:  socket.Data.SourcePort,
				Dest:     socket.Data.Dest,
				DstPort:  socket.Data.DestPort,
			}
		}
	}

	for _, entry := range sksB {
		for _, socket := range entry.OpenSockets {
			key := fmt.Sprintf("%d:%s:%s:%s:%d:%s:%d",
				entry.PID,
				socket.Protocol,
				socket.Data.Type,
				socket.Data.Address,
				socket.Data.SourcePort,
				socket.Data.Dest,
				socket.Data.DestPort,
			)
			mapB[key] = SocketInfo{
				PID:      int(entry.PID),
				Protocol: socket.Protocol,
				Type:     socket.Data.Type,
				Address:  socket.Data.Address,
				State:    socket.Data.State,
				Source:   socket.Data.Source,
				SrcPort:  socket.Data.SourcePort,
				Dest:     socket.Data.Dest,
				DstPort:  socket.Data.DestPort,
			}
		}
	}

	// Find differences
	for key, socketB := range mapB {
		if _, exists := mapA[key]; !exists {
			diff.Added = append(diff.Added, socketB)
		} else {
			diff.Unchanged = append(diff.Unchanged, socketB)
		}
	}

	for key, socketA := range mapA {
		if _, exists := mapB[key]; !exists {
			diff.Removed = append(diff.Removed, socketA)
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

	if result.SocketChanges != nil {
		added := len(result.SocketChanges.Added)
		removed := len(result.SocketChanges.Removed)

		if added > 0 || removed > 0 {
			summary += fmt.Sprintf("\nSockets: +%d -%d", added, removed)
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

	// Process tree
	if result.ProcessChanges != nil {
		fmt.Println("┌─ Process Tree ───────────────────────────────────────────────┐")

		added := len(result.ProcessChanges.Added)
		removed := len(result.ProcessChanges.Removed)
		modified := len(result.ProcessChanges.Modified)
		hasChanges := added+removed+modified > 0

		switch {
		case *showUnchanged && result.processTreeB != nil:
			status := buildProcessStatusMap(result.ProcessChanges)
			rendered := renderAnnotatedProcessTree(result.processTreeB, status)
			for _, line := range strings.Split(strings.TrimRight(rendered, "\n"), "\n") {
				fmt.Printf("│ %s\n", line)
			}
			if removed > 0 {
				fmt.Println("│ Removed:")
				for _, proc := range result.ProcessChanges.Removed {
					fmt.Printf("│   - PID %-5d %s\n", proc.PID, proc.Command)
				}
			}
		case !hasChanges:
			fmt.Println("│ = No change")
		default:
			if added > 0 {
				fmt.Println("│ Added:")
				for _, proc := range result.ProcessChanges.Added {
					fmt.Printf("│   + PID %-5d %s\n", proc.PID, proc.Command)
					if *psTreeCmd && proc.Cmdline != "" {
						fmt.Printf("│             %s\n", truncate(proc.Cmdline, 55))
					}
				}
			}
			if removed > 0 {
				fmt.Println("│ Removed:")
				for _, proc := range result.ProcessChanges.Removed {
					fmt.Printf("│   - PID %-5d %s\n", proc.PID, proc.Command)
				}
			}
			if modified > 0 {
				fmt.Println("│ Modified:")
				for _, proc := range result.ProcessChanges.Modified {
					fmt.Printf("│   ~ PID %-5d %s\n", proc.PID, proc.Command)
				}
			}
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

		if *showUnchanged && len(result.FileChanges.Unchanged) > 0 {
			fmt.Println("│ Unchanged:")
			for _, file := range result.FileChanges.Unchanged {
				fmt.Printf("│   = PID %-5d %-8s %s\n",
					file.PID, file.Type, truncate(file.Path, 38))
			}
		}
		fmt.Println("└──────────────────────────────────────────────────────────────┘")
	}

	// Socket changes
	if result.SocketChanges != nil {
		fmt.Println("┌─ Socket Changes ──────────────────────────────────────────────────────┐")

		hasChanges := len(result.SocketChanges.Added) > 0 || len(result.SocketChanges.Removed) > 0
		showUnchangedRows := *showUnchanged && len(result.SocketChanges.Unchanged) > 0

		if hasChanges || showUnchangedRows {
			fmt.Printf("│     %-5s %-5s %-12s %-21s %s\n",
				"PID", "PROTO", "STATE", "LOCAL", "PEER")
			for _, socket := range result.SocketChanges.Added {
				state, local, peer := formatSocketColumns(socket)
				fmt.Printf("│  +  %-5d %-5s %-12s %-21s %s\n",
					socket.PID, socket.Protocol, state, local, peer)
			}
			for _, socket := range result.SocketChanges.Removed {
				state, local, peer := formatSocketColumns(socket)
				fmt.Printf("│  -  %-5d %-5s %-12s %-21s %s\n",
					socket.PID, socket.Protocol, state, local, peer)
			}
			if showUnchangedRows {
				for _, socket := range result.SocketChanges.Unchanged {
					state, local, peer := formatSocketColumns(socket)
					fmt.Printf("│  =  %-5d %-5s %-12s %-21s %s\n",
						socket.PID, socket.Protocol, state, local, peer)
				}
			}
		}

		fmt.Println("└───────────────────────────────────────────────────────────────────────┘")
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

func formatSocketColumns(socket SocketInfo) (state, local, peer string) {
	switch socket.Type {
	case "TCP", "UDP":
		local = fmt.Sprintf("%s:%d", socket.Source, socket.SrcPort)
		if socket.DstPort > 0 {
			peer = fmt.Sprintf("%s:%d", socket.Dest, socket.DstPort)
		} else {
			peer = "-"
		}
		state = socket.State
		if state == "" {
			state = "-"
		}
	default:
		local = socket.Address
		if local == "" {
			local = "@"
		}
		state = "-"
		peer = "-"
	}
	return
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// buildProcessStatusMap indexes each PID in a ProcessDiff by its marker
// character: "+" added, "~" modified, "=" unchanged. Removed processes are not
// included because they do not appear in tree B.
func buildProcessStatusMap(diff *ProcessDiff) map[uint32]string {
	status := make(map[uint32]string)
	for _, p := range diff.Added {
		status[uint32(p.PID)] = "+"
	}
	for _, p := range diff.Modified {
		status[uint32(p.PID)] = "~"
	}
	for _, p := range diff.Unchanged {
		status[uint32(p.PID)] = "="
	}
	return status
}

// renderAnnotatedProcessTree walks tree and renders it via treeprint, prefixing
// each node with the marker from status (blank if the PID is unknown).
func renderAnnotatedProcessTree(tree *internal.PsNode, status map[uint32]string) string {
	root := treeprint.NewWithRoot("Process tree")
	addAnnotatedBranch(root, tree, status)
	return root.String()
}

func addAnnotatedBranch(parent treeprint.Tree, ps *internal.PsNode, status map[uint32]string) {
	if ps == nil {
		return
	}
	marker, ok := status[ps.PID]
	if !ok {
		marker = " "
	}
	displayName := ps.Comm
	if ps.Cmdline != "" {
		displayName = ps.Cmdline
	}
	branch := parent.AddMetaBranch(fmt.Sprintf("%s PID %d", marker, ps.PID), displayName)

	children := make([]internal.PsNode, len(ps.Children))
	copy(children, ps.Children)
	sort.Slice(children, func(i, j int) bool { return children[i].PID < children[j].PID })
	for i := range children {
		addAnnotatedBranch(branch, &children[i], status)
	}
}
