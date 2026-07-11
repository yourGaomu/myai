package history

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domainhistory "myai/core/domain/history"
	historyport "myai/core/port/history"
)

func TestRecorderDependsOnStoreContract(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, exists, err := SnapshotFile(path, "notes.txt")
	if err != nil || !exists {
		t.Fatalf("snapshot before failed: exists=%v err=%v", exists, err)
	}
	if err := os.WriteFile(path, []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &fakeHistoryStore{}
	recorder, err := NewRecorderWithStore(root, store)
	if err != nil {
		t.Fatal(err)
	}
	checkpointID, err := recorder.RecordFileChange(context.Background(), "notes.txt", &before, RecordCommand{Title: "edit notes"})
	if err != nil {
		t.Fatal(err)
	}
	if checkpointID != "checkpoint-1" || store.checkpoint.Title != "edit notes" || len(store.changes) != 1 {
		t.Fatalf("unexpected store command: checkpoint=%#v changes=%#v", store.checkpoint, store.changes)
	}
}

func TestTaskRecorderUsesStoreFactory(t *testing.T) {
	root := t.TempDir()
	factory := &fakeHistoryStoreFactory{store: &fakeHistoryStore{}}
	task := NewTaskRecorder(RecordCommand{Title: "task"}, factory)
	defer task.Close()

	workspaceRecorder, err := task.WorkspaceRecorder(root)
	if err != nil {
		t.Fatal(err)
	}
	if workspaceRecorder == nil || factory.openedPath == "" || factory.workspace == "" {
		t.Fatalf("expected factory-backed workspace recorder: %#v", factory)
	}
}

type fakeHistoryStoreFactory struct {
	store      historyport.Store
	workspace  string
	openedPath string
}

func (f *fakeHistoryStoreFactory) DefaultPath(workspace string) (string, error) {
	f.workspace = workspace
	return filepath.Join(workspace, "history.db"), nil
}

func (f *fakeHistoryStoreFactory) Open(path string) (historyport.Store, error) {
	f.openedPath = path
	return f.store, nil
}

type fakeHistoryStore struct {
	checkpoint domainhistory.Checkpoint
	changes    []domainhistory.FileChange
}

func (*fakeHistoryStore) Close() error {
	return nil
}

func (*fakeHistoryStore) HasBaseline(context.Context, string) (bool, error) {
	return false, nil
}

func (*fakeHistoryStore) LoadBaseline(context.Context, string) (map[string]domainhistory.FileSnapshot, error) {
	return nil, nil
}

func (*fakeHistoryStore) ReplaceBaseline(context.Context, string, map[string]domainhistory.FileSnapshot) error {
	return nil
}

func (s *fakeHistoryStore) SaveCheckpoint(_ context.Context, checkpoint domainhistory.Checkpoint, changes []domainhistory.FileChange) (string, error) {
	s.checkpoint = checkpoint
	s.changes = append([]domainhistory.FileChange(nil), changes...)
	return "checkpoint-1", nil
}

func (*fakeHistoryStore) ListCheckpoints(context.Context, string, int) ([]domainhistory.CheckpointSummary, error) {
	return nil, nil
}

func (*fakeHistoryStore) LoadCheckpointChanges(context.Context, string, string) ([]domainhistory.StoredFileChange, error) {
	return nil, nil
}
