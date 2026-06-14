package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"myai/core/history"
	"myai/core/sandbox"
)

func TestWriteFileRecordsCheckpoint(t *testing.T) {
	root := t.TempDir()
	store := newTestHistoryStore(t)
	recorder := newTestRecorder(t, root, store)
	tool := NewWriteFileToolWithWorkspaceAndRecorder(root, recorder)

	output, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"path":    "notes.txt",
		"content": "hello\n",
	}))
	if err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	var result writeFileResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode write result failed: %v", err)
	}
	if result.CheckpointID == "" {
		t.Fatalf("expected checkpoint id, output=%s", output)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); err != nil {
		t.Fatalf("expected file in configured workspace: %v", err)
	}
	assertCheckpointCount(t, store, root, 1)
}

func TestEditFileRecordsCheckpoint(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}

	store := newTestHistoryStore(t)
	recorder := newTestRecorder(t, root, store)
	tool := NewEditFileToolWithWorkspaceAndRecorder(root, recorder)

	output, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"path":     filepath.Join(root, "main.go"),
		"old_text": "package main\n",
		"new_text": "package main\n\nfunc main() {}\n",
	}))
	if err != nil {
		t.Fatalf("edit file failed: %v", err)
	}

	var result editFileResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode edit result failed: %v", err)
	}
	if result.CheckpointID == "" {
		t.Fatalf("expected checkpoint id, output=%s", output)
	}
	assertCheckpointCount(t, store, root, 1)
}

func TestTaskRecorderGroupsMultipleFileChanges(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}

	store := newTestHistoryStore(t)
	task := history.NewTaskRecorderWithStore(history.RecordOptions{
		Title:     "implement feature",
		Reason:    "user request",
		SessionID: "session-1",
		RequestID: "request-1",
	}, store)
	defer task.Close()

	ctx := history.WithTaskRecorder(context.Background(), task)
	writeTool := NewWriteFileToolWithWorkspace(root)
	editTool := NewEditFileToolWithWorkspace(root)

	if _, err := editTool.Call(ctx, mustJSON(t, map[string]any{
		"path":     "main.go",
		"old_text": "package main\n",
		"new_text": "package main\n\nfunc main() {}\n",
	})); err != nil {
		t.Fatalf("edit file failed: %v", err)
	}
	if _, err := writeTool.Call(ctx, mustJSON(t, map[string]any{
		"path":    "README.md",
		"content": "# Demo\n",
	})); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	ids, err := task.Save(context.Background())
	if err != nil {
		t.Fatalf("save task recorder failed: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected one task checkpoint, got %+v", ids)
	}

	assertCheckpointCountWithChanges(t, store, root, 1, 2)
	changes, err := store.LoadCheckpointChanges(context.Background(), absPath(t, root), ids[0])
	if err != nil {
		t.Fatalf("load checkpoint changes failed: %v", err)
	}
	if !hasStoredChange(changes, "main.go", "modified") {
		t.Fatalf("expected modified main.go change, got %+v", changes)
	}
	if !hasStoredChange(changes, "README.md", "added") {
		t.Fatalf("expected added README.md change, got %+v", changes)
	}
}

func TestTaskRecorderMergesRepeatedFileChanges(t *testing.T) {
	root := t.TempDir()

	store := newTestHistoryStore(t)
	task := history.NewTaskRecorderWithStore(history.RecordOptions{
		Title: "repeat file",
	}, store)
	defer task.Close()

	ctx := history.WithTaskRecorder(context.Background(), task)
	writeTool := NewWriteFileToolWithWorkspace(root)
	editTool := NewEditFileToolWithWorkspace(root)

	if _, err := writeTool.Call(ctx, mustJSON(t, map[string]any{
		"path":    "notes.txt",
		"content": "hello\n",
	})); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	if _, err := editTool.Call(ctx, mustJSON(t, map[string]any{
		"path":     "notes.txt",
		"old_text": "hello\n",
		"new_text": "hello world\n",
	})); err != nil {
		t.Fatalf("edit file failed: %v", err)
	}

	ids, err := task.Save(context.Background())
	if err != nil {
		t.Fatalf("save task recorder failed: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected one task checkpoint, got %+v", ids)
	}
	changes, err := store.LoadCheckpointChanges(context.Background(), absPath(t, root), ids[0])
	if err != nil {
		t.Fatalf("load checkpoint changes failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected merged one file change, got %+v", changes)
	}
	if changes[0].Path != "notes.txt" || changes[0].ChangeType != "added" {
		t.Fatalf("expected merged added notes.txt change, got %+v", changes[0])
	}
	if changes[0].After == nil || string(changes[0].After.Content) != "hello world\n" {
		t.Fatalf("expected final file content, got %+v", changes[0].After)
	}
}

func TestShellRecordsWorkspaceChangesInTaskCheckpoint(t *testing.T) {
	root := t.TempDir()
	writeGoFile := `package main

import "os"

func main() {
	_ = os.WriteFile("generated.txt", []byte("generated\n"), 0644)
	_ = os.WriteFile("existing.txt", []byte("changed\n"), 0644)
}
`
	if err := os.WriteFile(filepath.Join(root, "script.go"), []byte(writeGoFile), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write seed file failed: %v", err)
	}

	store := newTestHistoryStore(t)
	task := history.NewTaskRecorderWithStore(history.RecordOptions{
		Title: "run generator",
	}, store)
	defer task.Close()

	localSandbox, err := sandbox.NewLocalSandbox(root)
	if err != nil {
		t.Fatalf("new local sandbox failed: %v", err)
	}
	tool := NewShellToolWithWorkspace(root, localSandbox)
	ctx := history.WithTaskRecorder(context.Background(), task)

	output, err := tool.Call(ctx, mustJSON(t, map[string]any{
		"command": "go run script.go",
	}))
	if err != nil {
		t.Fatalf("shell failed: %v\n%s", err, output)
	}

	ids, err := task.Save(context.Background())
	if err != nil {
		t.Fatalf("save task recorder failed: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected one task checkpoint, got %+v", ids)
	}

	changes, err := store.LoadCheckpointChanges(context.Background(), absPath(t, root), ids[0])
	if err != nil {
		t.Fatalf("load checkpoint changes failed: %v", err)
	}
	if !hasStoredChange(changes, "generated.txt", "added") {
		t.Fatalf("expected added generated.txt change, got %+v", changes)
	}
	if !hasStoredChange(changes, "existing.txt", "modified") {
		t.Fatalf("expected modified existing.txt change, got %+v", changes)
	}
	if hasStoredChange(changes, "script.go", "modified") {
		t.Fatalf("script.go should not be recorded as modified, got %+v", changes)
	}
}

func newTestHistoryStore(t *testing.T) *history.SQLiteStore {
	t.Helper()

	store, err := history.OpenSQLite(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open history store failed: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close history store failed: %v", err)
		}
	})
	return store
}

func newTestRecorder(t *testing.T, root string, store *history.SQLiteStore) *history.Recorder {
	t.Helper()

	recorder, err := history.NewRecorderWithStore(root, store)
	if err != nil {
		t.Fatalf("new recorder failed: %v", err)
	}
	return recorder
}

func assertCheckpointCount(t *testing.T, store *history.SQLiteStore, workspace string, expected int) {
	t.Helper()
	assertCheckpointCountWithChanges(t, store, workspace, expected, 1)
}

func assertCheckpointCountWithChanges(t *testing.T, store *history.SQLiteStore, workspace string, expected int, expectedChanges int) {
	t.Helper()

	checkpoints, err := store.ListCheckpoints(context.Background(), absPath(t, workspace), 20)
	if err != nil {
		t.Fatalf("list checkpoints failed: %v", err)
	}
	if len(checkpoints) != expected {
		t.Fatalf("expected %d checkpoints, got %d: %+v", expected, len(checkpoints), checkpoints)
	}
	if checkpoints[0].ChangeCount != expectedChanges {
		t.Fatalf("expected %d file change(s), got %+v", expectedChanges, checkpoints[0])
	}
}

func absPath(t *testing.T, path string) string {
	t.Helper()

	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs path failed: %v", err)
	}
	return abs
}

func hasStoredChange(changes []history.StoredFileChange, path string, changeType string) bool {
	for _, change := range changes {
		if change.Path == path && change.ChangeType == changeType {
			return true
		}
	}
	return false
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json failed: %v", err)
	}
	return data
}
