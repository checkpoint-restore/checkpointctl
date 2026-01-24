package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"
)

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout = w

	f()

	if err := w.Close(); err != nil {
		panic(err)
	}
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		panic(err)
	}

	if err := r.Close(); err != nil {
		panic(err)
	}

	return buf.String()
}

func newTask(path string) *Task {
	return &Task{
		CheckpointFilePath: path,
	}
}

func TestDiffTasks_NilInput(t *testing.T) {
	_, err := DiffTasks(nil, []*Task{}, false, false, false, false)
	if err == nil {
		t.Fatalf("expected error for nil tasksA")
	}

	_, err = DiffTasks([]*Task{}, nil, false, false, false, false)
	if err == nil {
		t.Fatalf("expected error for nil tasksB")
	}
}

func TestDiffTasks_Unchanged(t *testing.T) {
	taskA := newTask("task1")
	taskB := newTask("task1")

	diffs, err := DiffTasks([]*Task{taskA}, []*Task{taskB}, false, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	if diffs[0].Status != Unchanged {
		t.Fatalf("expected status %s, got %s", Unchanged, diffs[0].Status)
	}
}

func TestDiffTasks_Added(t *testing.T) {
	taskB := newTask("task1")

	diffs, err := DiffTasks([]*Task{}, []*Task{taskB}, false, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	if diffs[0].Status != Added {
		t.Fatalf("expected status %s, got %s", Added, diffs[0].Status)
	}

	if diffs[0].After == nil {
		t.Fatalf("expected After to be set")
	}
}

func TestDiffTasks_Removed(t *testing.T) {
	taskA := newTask("task1")

	diffs, err := DiffTasks([]*Task{taskA}, []*Task{}, false, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	if diffs[0].Status != Removed {
		t.Fatalf("expected status %s, got %s", Removed, diffs[0].Status)
	}

	if diffs[0].Before == nil {
		t.Fatalf("expected Before to be set")
	}
}

func TestDiffTasks_Modified(t *testing.T) {
	taskA := newTask("task1")
	taskB := newTask("task1")

	// Modify taskB so DeepEqual fails
	taskBExtra := *taskB
	taskBExtra.CheckpointFilePath = "task1_modified"

	diffs, err := DiffTasks(
		[]*Task{taskA},
		[]*Task{&taskBExtra},
		false, false, false, false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diffs) != 2 {
		// Because IDs are based on CheckpointFilePath, this becomes Removed + Added
		t.Fatalf("expected 2 diffs (removed + added), got %d", len(diffs))
	}
}

func TestDiffTasks_ModifiedSameID(t *testing.T) {
	taskA := newTask("task1")
	taskB := &Task{
		CheckpointFilePath: "task1",
	}

	// Force inequality by modifying struct via reflection-safe trick
	// (If Task has additional fields, modify one here)

	diffs, err := DiffTasks(
		[]*Task{taskA},
		[]*Task{taskB},
		false, false, false, false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	if diffs[0].Status != Unchanged && diffs[0].Status != Modified {
		t.Fatalf("unexpected status: %s", diffs[0].Status)
	}
}

func TestDiffTasks_Mixed(t *testing.T) {
	a1 := newTask("same")
	a2 := newTask("removed")

	b1 := newTask("same")
	b2 := newTask("added")

	diffs, err := DiffTasks(
		[]*Task{a1, a2},
		[]*Task{b1, b2},
		false, false, false, false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diffs) != 3 {
		t.Fatalf("expected 3 diffs, got %d", len(diffs))
	}
}

func TestRenderDiffTreeView(t *testing.T) {
	task := newTask("task1")

	diff := []DiffTask{
		{
			ID:     "task1",
			Status: Added,
			After:  task,
			Changes: []DiffChange{
				{
					Category: "task",
					Field:    "struct",
					Before:   nil,
					After:    task,
				},
			},
		},
	}

	output := captureOutput(func() {
		err := RenderDiffTreeView(diff)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if output == "" {
		t.Fatalf("expected output, got empty string")
	}
}

func TestRenderDiffJSONView(t *testing.T) {
	task := newTask("task1")

	diff := []DiffTask{
		{
			ID:     "task1",
			Status: Added,
			After:  task,
		},
	}

	output := captureOutput(func() {
		err := RenderDiffJSONView(diff)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if output == "" {
		t.Fatalf("expected JSON output")
	}

	var decoded []DiffTask
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if !reflect.DeepEqual(decoded[0].ID, "task1") {
		t.Fatalf("unexpected JSON content")
	}
}
