package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domainhistory "myai/core/domain/history"
	historyport "myai/core/port/history"
)

func TestStoreImplementsHistoryPort(t *testing.T) {
	var _ historyport.Store = (*Store)(nil)
	var _ historyport.StoreFactory = Factory{}
}

func TestStorePersistsBaselineAndCheckpoint(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	workspace := filepath.Clean(t.TempDir())
	snapshot := domainhistory.FileSnapshot{
		Path:      "main.go",
		Size:      4,
		Content:   []byte("main"),
		Mode:      os.FileMode(0o644),
		Available: true,
	}
	if err := store.ReplaceBaseline(context.Background(), workspace, map[string]domainhistory.FileSnapshot{
		"main.go": snapshot,
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadBaseline(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if loaded["main.go"].Path != "main.go" || string(loaded["main.go"].Content) != "main" {
		t.Fatalf("unexpected baseline: %#v", loaded)
	}

	checkpointID, err := store.SaveCheckpoint(context.Background(), domainhistory.Checkpoint{
		Workspace: workspace,
		Title:     "edit",
	}, []domainhistory.FileChange{{
		Path:       "main.go",
		ChangeType: "modified",
		After:      &snapshot,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if checkpointID == "" {
		t.Fatal("expected generated checkpoint id")
	}
	changes, err := store.LoadCheckpointChanges(context.Background(), workspace, checkpointID)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Path != "main.go" {
		t.Fatalf("unexpected changes: %#v", changes)
	}
}
