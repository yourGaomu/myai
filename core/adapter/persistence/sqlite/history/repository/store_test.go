package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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
	if err := store.DeleteCheckpoint(context.Background(), workspace, checkpointID); err != nil {
		t.Fatal(err)
	}
	checkpoints, err := store.ListCheckpoints(context.Background(), workspace, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) != 0 {
		t.Fatalf("expected checkpoint deletion, got %#v", checkpoints)
	}
}

func TestCheckpointDoesNotInitializeBaseline(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	workspace := t.TempDir()
	snapshot := storeTestSnapshot("notes.txt")
	if _, err := store.SaveCheckpoint(ctx, domainhistory.Checkpoint{Workspace: workspace}, []domainhistory.FileChange{{Path: snapshot.Path, After: &snapshot}}); err != nil {
		t.Fatal(err)
	}
	exists, err := store.HasBaseline(ctx, workspace)
	if err != nil || exists {
		t.Fatalf("checkpoint initialized a baseline: exists=%v err=%v", exists, err)
	}
	if err := store.ReplaceBaseline(ctx, workspace, map[string]domainhistory.FileSnapshot{}); err != nil {
		t.Fatal(err)
	}
	exists, err = store.HasBaseline(ctx, workspace)
	if err != nil || !exists {
		t.Fatalf("empty baseline was not marked: exists=%v err=%v", exists, err)
	}
}

func TestLegacyBaselineMarkerMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, workspace := range []string{"nonempty", "empty", "checkpoint-only"} {
		if err := store.ReplaceBaseline(ctx, workspace, nil); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := storeTestSnapshot("notes.txt")
	if err := store.ReplaceBaseline(ctx, "nonempty", map[string]domainhistory.FileSnapshot{snapshot.Path: snapshot}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveCheckpoint(ctx, domainhistory.Checkpoint{Workspace: "checkpoint-only"}, []domainhistory.FileChange{{Path: snapshot.Path, After: &snapshot}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Remove the new table to reproduce an old database with workspace rows only.
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`DROP TABLE workspace_baselines`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		store, err = Open(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, workspace := range []string{"nonempty", "empty", "checkpoint-only"} {
			exists, err := store.HasBaseline(ctx, workspace)
			if err != nil || exists != (workspace == "nonempty") {
				t.Fatalf("migration pass %d workspace %s: exists=%v err=%v", pass, workspace, exists, err)
			}
		}
		baseline, err := store.LoadBaseline(ctx, "nonempty")
		if err != nil || string(baseline["notes.txt"].Content) != "fixture" {
			t.Fatal("migration changed old baseline")
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFactoryBaseDirKeepsHistoryUnderConfiguredDirectory(t *testing.T) {
	workspace, base := t.TempDir(), t.TempDir()
	legacy, err := DefaultPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	compatible, err := (Factory{}).DefaultPath(workspace)
	if err != nil || compatible != legacy {
		t.Fatal("empty factory changed the legacy default")
	}
	path, err := (Factory{BaseDir: base}).DefaultPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(base, "workspaces")+string(filepath.Separator)) || filepath.Base(path) != "history.db" {
		t.Fatalf("history path escaped configured base: %s", path)
	}
	if filepath.Base(filepath.Dir(path)) != filepath.Base(filepath.Dir(legacy)) {
		t.Fatal("configured base changed workspace identity")
	}
	store, err := (Factory{BaseDir: base}).Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestStoreDoesNotPersistProtectedSnapshots(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	files := make(map[string]domainhistory.FileSnapshot)
	var changes []domainhistory.FileChange
	for _, name := range []string{".env.staging", "nested/Application.YAML", `nested\relay.yaml`, "credentials/private.pem", ".git/config", "notes.txt"} {
		file := storeTestSnapshot(name)
		files[name] = file
		changes = append(changes, domainhistory.FileChange{Path: name, After: &file})
	}
	if err := store.ReplaceBaseline(ctx, "workspace", files); err != nil {
		t.Fatal(err)
	}
	id, err := store.SaveCheckpoint(ctx, domainhistory.Checkpoint{Workspace: "workspace"}, changes)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"workspace_baseline_files", "file_changes"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("protected content persisted in %s: %d rows", table, count)
		}
	}
	loaded, err := store.LoadCheckpointChanges(ctx, "workspace", id)
	if err != nil || len(loaded) != 1 || loaded[0].Path != "notes.txt" {
		t.Fatal("expected only ordinary file in checkpoint")
	}
}

func TestReplaceBaselineFailurePreservesMarkerAndFiles(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	file := storeTestSnapshot("notes.txt")
	if err := store.ReplaceBaseline(ctx, "workspace", map[string]domainhistory.FileSnapshot{file.Path: file}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_baseline BEFORE INSERT ON workspace_baseline_files BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceBaseline(ctx, "workspace", map[string]domainhistory.FileSnapshot{file.Path: file}); err == nil {
		t.Fatal("expected injected baseline write failure")
	}
	exists, err := store.HasBaseline(ctx, "workspace")
	if err != nil || !exists {
		t.Fatal("baseline failure lost marker")
	}
	loaded, err := store.LoadBaseline(ctx, "workspace")
	if err != nil || len(loaded) != 1 {
		t.Fatal("baseline failure lost original files")
	}
}

func storeTestSnapshot(name string) domainhistory.FileSnapshot {
	return domainhistory.FileSnapshot{Path: name, Content: []byte("fixture"), Size: 7, Hash: sha256.Sum256([]byte("fixture")), Mode: 0o644, Available: true}
}
