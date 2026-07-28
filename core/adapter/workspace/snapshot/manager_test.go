package snapshot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainhistory "myai/core/domain/history"
	domainworkspace "myai/core/domain/workspace"
	historyport "myai/core/port/history"
	workspaceport "myai/core/port/workspace"
)

func TestSnapshotWorkspaceCollectsAndAppliesChanges(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, source, "main.go", "package main\n")
	store := &fakeHistoryStore{}
	manager, err := New(t.TempDir(), fakeHistoryFactory{store: store})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{
		WorkspaceID: "workspace-1", Mode: domainworkspace.IsolationModeSnapshot, SourceRoot: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, prepared.Reference.Root, "main.go", "package main\n\nfunc main() {}\n")
	writeTestFile(t, prepared.Reference.Root, "new.txt", "new\n")

	collected, err := manager.Collect(context.Background(), workspaceport.CollectRequest{Reference: prepared.Reference})
	if err != nil {
		t.Fatal(err)
	}
	if collected.ChangeSet.Status != domainworkspace.ChangeSetStatusPending || len(collected.ChangeSet.Files) != 2 {
		t.Fatalf("unexpected change set: %#v", collected.ChangeSet)
	}
	assertFileContent(t, filepath.Join(source, "main.go"), "package main\n")

	applied, err := manager.Apply(context.Background(), workspaceport.ApplyRequest{
		Reference: prepared.Reference, TaskID: "task-1", SessionID: "session-1", Title: "apply task",
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.ChangeSet.Status != domainworkspace.ChangeSetStatusApplied || applied.ChangeSet.CheckpointID != "checkpoint-1" {
		t.Fatalf("unexpected applied changes: %#v", applied.ChangeSet)
	}
	assertFileContent(t, filepath.Join(source, "main.go"), "package main\n\nfunc main() {}\n")
	assertFileContent(t, filepath.Join(source, "new.txt"), "new\n")
	if len(store.changes) != 2 {
		t.Fatalf("expected two checkpoint changes, got %d", len(store.changes))
	}
}

func TestSnapshotWorkspaceRejectsSourceConflict(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, source, "main.go", "before\n")
	manager, err := New(t.TempDir(), fakeHistoryFactory{store: &fakeHistoryStore{}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{
		WorkspaceID: "workspace-2", Mode: domainworkspace.IsolationModeSnapshot, SourceRoot: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, prepared.Reference.Root, "main.go", "subagent\n")
	writeTestFile(t, source, "main.go", "user change\n")

	result, err := manager.Apply(context.Background(), workspaceport.ApplyRequest{Reference: prepared.Reference})
	if err == nil || result.ChangeSet.Status != domainworkspace.ChangeSetStatusConflict {
		t.Fatalf("expected conflict result, result=%#v err=%v", result, err)
	}
	assertFileContent(t, filepath.Join(source, "main.go"), "user change\n")
}

func TestSnapshotWorkspaceDeletesCheckpointWhenManifestCommitFails(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, source, "main.go", "before\n")
	historyStore := &fakeHistoryStore{}
	manager, err := New(t.TempDir(), fakeHistoryFactory{store: historyStore})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{
		WorkspaceID: "workspace-manifest-failure", Mode: domainworkspace.IsolationModeSnapshot, SourceRoot: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, prepared.Reference.Root, "main.go", "after\n")
	manager.persistManifest = func(string, manifest) error {
		return errors.New("manifest write failed")
	}

	if _, err := manager.Apply(context.Background(), workspaceport.ApplyRequest{Reference: prepared.Reference}); err == nil {
		t.Fatal("expected manifest failure")
	}
	assertFileContent(t, filepath.Join(source, "main.go"), "before\n")
	if historyStore.deleted != "checkpoint-1" {
		t.Fatalf("expected checkpoint compensation, got %q", historyStore.deleted)
	}
}

func TestSnapshotWorkspaceDiscardRemovesIsolatedFiles(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, source, "main.go", "before\n")
	manager, err := New(t.TempDir(), fakeHistoryFactory{store: &fakeHistoryStore{}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{
		WorkspaceID: "workspace-3", Mode: domainworkspace.IsolationModeSnapshot, SourceRoot: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Discard(context.Background(), workspaceport.DiscardRequest{Reference: prepared.Reference})
	if err != nil {
		t.Fatal(err)
	}
	if result.ChangeSet.Status != domainworkspace.ChangeSetStatusDiscarded {
		t.Fatalf("unexpected discard result: %#v", result.ChangeSet)
	}
	if _, err := os.Stat(prepared.Reference.Root); !os.IsNotExist(err) {
		t.Fatalf("expected snapshot root to be removed, err=%v", err)
	}
}

func TestSnapshotWorkspaceStoredInsideSourceDoesNotCopyRuntimeDirectory(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, source, "main.go", "package main\n")
	writeTestFile(t, source, ".myai/knowledge.db", "runtime data")
	root := filepath.Join(source, ".myai", "subagent-snapshots")
	manager, err := New(root, fakeHistoryFactory{store: &fakeHistoryStore{}})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{
		WorkspaceID: "workspace-inside-source", Mode: domainworkspace.IsolationModeSnapshot, SourceRoot: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(prepared.Reference.Root, "main.go"), "package main\n")
	if _, err := os.Stat(filepath.Join(prepared.Reference.Root, ".myai")); !os.IsNotExist(err) {
		t.Fatalf("runtime directory must not be copied into snapshot, err=%v", err)
	}

	value, err := loadManifest(filepath.Join(root, "workspace-inside-source"))
	if err != nil {
		t.Fatal(err)
	}
	if len(value.BaselineFiles) != 1 || value.BaselineFiles[0].Path != "main.go" {
		t.Fatalf("unexpected snapshot baseline: %#v", value.BaselineFiles)
	}
}

func writeTestFile(t *testing.T, root string, relative string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != expected {
		t.Fatalf("expected %q, got %q", expected, string(content))
	}
}

type fakeHistoryFactory struct{ store historyport.Store }

func (factory fakeHistoryFactory) Open(string) (historyport.Store, error) { return factory.store, nil }
func (fakeHistoryFactory) DefaultPath(string) (string, error)             { return "history.db", nil }

type fakeHistoryStore struct {
	checkpoint domainhistory.Checkpoint
	changes    []domainhistory.FileChange
	deleted    string
}

func (*fakeHistoryStore) Close() error { return nil }
func (*fakeHistoryStore) HasBaseline(context.Context, string) (bool, error) {
	return false, nil
}
func (*fakeHistoryStore) LoadBaseline(context.Context, string) (map[string]domainhistory.FileSnapshot, error) {
	return nil, nil
}
func (*fakeHistoryStore) ReplaceBaseline(context.Context, string, map[string]domainhistory.FileSnapshot) error {
	return nil
}
func (store *fakeHistoryStore) SaveCheckpoint(_ context.Context, checkpoint domainhistory.Checkpoint, changes []domainhistory.FileChange) (string, error) {
	store.checkpoint = checkpoint
	store.changes = append([]domainhistory.FileChange(nil), changes...)
	return "checkpoint-1", nil
}
func (store *fakeHistoryStore) DeleteCheckpoint(_ context.Context, _ string, checkpointID string) error {
	store.deleted = checkpointID
	return nil
}
func (*fakeHistoryStore) ListCheckpoints(context.Context, string, int) ([]domainhistory.CheckpointSummary, error) {
	return nil, nil
}
func (*fakeHistoryStore) LoadCheckpointChanges(context.Context, string, string) ([]domainhistory.StoredFileChange, error) {
	return nil, nil
}
