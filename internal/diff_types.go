package internal

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// DiffStatus describes how a task changed between two checkpoints
type DiffStatus string

const (
	Added     DiffStatus = "added"
	Removed   DiffStatus = "removed"
	Modified  DiffStatus = "modified"
	Unchanged DiffStatus = "unchanged"
)

// DiffTask represents the forensic diff of a single task/process
type DiffTask struct {
	// Stable identifier for matching tasks across checkpoints
	ID string `json:"id"`

	// High-level classification of the change
	Status DiffStatus `json:"status"`

	// Task state in checkpoint A (nil if Added)
	Before *Task `json:"before,omitempty"`

	// Task state in checkpoint B (nil if Removed)
	After *Task `json:"after,omitempty"`

	// Fine-grained differences detected inside the task
	Changes []DiffChange `json:"changes,omitempty"`
}

// DiffChange represents a single detected difference.
type DiffChange struct {
	// Logical category of the change (pstree, files, sockets, env, cmdline, etc.)
	Category string `json:"category"`

	// Specific field or subcomponent that changed
	Field string `json:"field"`

	// Value in checkpoint A
	Before any `json:"before,omitempty"`

	// Value in checkpoint B
	After any `json:"after,omitempty"`
}

// DiffTasks compares two sets of tasks and returns their differences.
func DiffTasks(
	tasksA []*Task,
	tasksB []*Task,
	psTreeCmd bool,
	psTreeEnv bool,
	files bool,
	sockets bool,
) ([]DiffTask, error) {
	if tasksA == nil || tasksB == nil {
		return nil, fmt.Errorf("nil task list provided")
	}

	// Index tasks by CheckpointFilePath for matching
	indexA := make(map[string]*Task)
	indexB := make(map[string]*Task)

	for _, t := range tasksA {
		indexA[t.CheckpointFilePath] = t
	}
	for _, t := range tasksB {
		indexB[t.CheckpointFilePath] = t
	}

	var diffs []DiffTask

	// Tasks present in A
	for id, taskA := range indexA {
		taskB, exists := indexB[id]

		if !exists {
			// Removed task
			diffs = append(diffs, DiffTask{
				ID:     id,
				Status: Removed,
				Before: taskA,
			})
			continue
		}

		// Exists in both → compare
		if reflect.DeepEqual(taskA, taskB) {
			diffs = append(diffs, DiffTask{
				ID:     id,
				Status: Unchanged,
				Before: taskA,
				After:  taskB,
			})
			continue
		}

		// Modified task
		diffs = append(diffs, DiffTask{
			ID:     id,
			Status: Modified,
			Before: taskA,
			After:  taskB,
			Changes: []DiffChange{
				{
					Category: "task",
					Field:    "struct",
					Before:   taskA,
					After:    taskB,
				},
			},
		})
	}

	// Tasks only in B → Added
	for id, taskB := range indexB {
		if _, exists := indexA[id]; exists {
			continue
		}

		diffs = append(diffs, DiffTask{
			ID:     id,
			Status: Added,
			After:  taskB,
		})
	}

	return diffs, nil
}

// RenderDiffTreeView prints a human-readable tree of diff tasks
func RenderDiffTreeView(diffTasks []DiffTask) error {
	for _, dt := range diffTasks {
		fmt.Printf("\nTask ID: %s | Status: %s\n", dt.ID, dt.Status)

		if dt.Before != nil {
			fmt.Printf("  Before checkpoint: %s\n", dt.Before.CheckpointFilePath)
		}
		if dt.After != nil {
			fmt.Printf("  After checkpoint: %s\n", dt.After.CheckpointFilePath)
		}

		for _, ch := range dt.Changes {
			fmt.Printf("  Change: [%s] %s | Before: %v | After: %v\n",
				ch.Category, ch.Field, ch.Before, ch.After)
		}
	}
	return nil
}

// RenderDiffJSONView prints diff tasks in JSON format
func RenderDiffJSONView(diffTasks []DiffTask) error {
	jsonData, err := json.MarshalIndent(diffTasks, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(jsonData))
	return nil
}
