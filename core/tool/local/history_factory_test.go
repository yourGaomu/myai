package local

import (
	"context"
	"path/filepath"
	"testing"

	sqlitehistory "myai/core/adapter/persistence/sqlite/history/repository"
)

func TestFileToolsUseConfiguredHistoryFactory(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	base := t.TempDir()
	factory := sqlitehistory.Factory{BaseDir: base}
	write := NewWriteFileToolWithWorkspaceAndHistory(workspace, factory)
	if _, err := write.Call(ctx, mustJSON(t, map[string]any{"path": "notes.txt", "content": "first\n"})); err != nil {
		t.Fatal(err)
	}
	edit := NewEditFileToolWithWorkspaceAndHistory(workspace, factory)
	if _, err := edit.Call(ctx, mustJSON(t, map[string]any{"path": "notes.txt", "old_text": "first", "new_text": "second"})); err != nil {
		t.Fatal(err)
	}
	historyPath, err := factory.DefaultPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(base, historyPath)
	if err != nil || !filepath.IsLocal(relative) {
		t.Fatalf("history must use the configured persistent directory: %v", err)
	}
	store, err := factory.Open(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	checkpoints, err := store.ListCheckpoints(ctx, absPath(t, workspace), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) != 2 {
		t.Fatalf("write and edit must share the configured history database, got %d checkpoints", len(checkpoints))
	}
}
