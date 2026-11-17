package changes

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqlitehistory "myai/core/adapter/persistence/sqlite/history/repository"
	domainhistory "myai/core/domain/history"
	"myai/core/history"
	"myai/core/remote/protocol"
)

func TestListDiffAndRevertSnapshotChanges(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "package main\n")
	writeTestFile(t, root, "keep.txt", "same\n")

	service := newTestService(t, root)

	writeTestFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	writeTestFile(t, root, "new.txt", "hello\n")
	if err := os.Remove(filepath.Join(root, "keep.txt")); err != nil {
		t.Fatalf("remove file failed: %v", err)
	}

	result, err := service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil {
		t.Fatalf("list changes failed: %v", err)
	}
	if result.Clean {
		t.Fatalf("expected dirty workspace")
	}
	if result.Source != "sqlite" {
		t.Fatalf("expected sqlite source, got %q", result.Source)
	}
	if !hasChange(result.Entries, "main.go") {
		t.Fatalf("expected main.go change: %+v", result.Entries)
	}
	if !hasChange(result.Entries, "new.txt") {
		t.Fatalf("expected new.txt change: %+v", result.Entries)
	}
	if !hasChange(result.Entries, "keep.txt") {
		t.Fatalf("expected keep.txt deletion: %+v", result.Entries)
	}

	diff, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: "main.go"})
	if err != nil {
		t.Fatalf("diff changed file failed: %v", err)
	}
	if !diff.Restorable {
		t.Fatalf("expected changed file to be restorable")
	}
	if !strings.Contains(diff.Diff, "+func main() {}") {
		t.Fatalf("expected main.go diff, got:\n%s", diff.Diff)
	}

	reverted, err := service.Revert(context.Background(), protocol.ChangeRevertPayload{Path: "main.go", ExpectedHash: diff.ExpectedHash})
	if err != nil {
		t.Fatalf("revert changed file failed: %v", err)
	}
	if !reverted.Reverted {
		t.Fatalf("expected reverted=true")
	}
	content, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatalf("read reverted file failed: %v", err)
	}
	if string(content) != "package main\n" {
		t.Fatalf("expected original content, got %q", string(content))
	}
}

func newTestServiceWithHistoryPath(root string, historyPath string) (*Service, error) {
	return NewWithStoreFactory(root, historyPath, sqlitehistory.Factory{})
}

func TestDiffNewFileDoesNotRequireGit(t *testing.T) {
	root := t.TempDir()
	service := newTestService(t, root)

	writeTestFile(t, root, "new.txt", "hello\n")

	diff, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: "new.txt"})
	if err != nil {
		t.Fatalf("diff new file failed: %v", err)
	}
	if !diff.Restorable {
		t.Fatalf("new files should be restorable by removing them")
	}
	if !strings.Contains(diff.Diff, "+hello") {
		t.Fatalf("expected new file diff, got:\n%s", diff.Diff)
	}
}

func TestRevertDeletedFileRestoresSnapshot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "gone.txt", "restore me\n")
	service := newTestService(t, root)

	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		t.Fatalf("remove file failed: %v", err)
	}

	diff, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: "gone.txt"})
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := service.Revert(context.Background(), protocol.ChangeRevertPayload{Path: "gone.txt", ExpectedHash: diff.ExpectedHash})
	if err != nil {
		t.Fatalf("revert deleted file failed: %v", err)
	}
	if !reverted.Reverted {
		t.Fatalf("expected reverted=true")
	}
	content, err := os.ReadFile(filepath.Join(root, "gone.txt"))
	if err != nil {
		t.Fatalf("read restored file failed: %v", err)
	}
	if string(content) != "restore me\n" {
		t.Fatalf("expected restored content, got %q", string(content))
	}
}

func TestRevertNewFileRemovesIt(t *testing.T) {
	root := t.TempDir()
	service := newTestService(t, root)
	writeTestFile(t, root, "new.txt", "hello\n")

	diff, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: "new.txt"})
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := service.Revert(context.Background(), protocol.ChangeRevertPayload{Path: "new.txt", ExpectedHash: diff.ExpectedHash})
	if err != nil {
		t.Fatalf("revert new file failed: %v", err)
	}
	if !reverted.Reverted {
		t.Fatalf("expected reverted=true")
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected new file to be removed, err=%v", err)
	}
}

func TestSQLiteBaselineSurvivesServiceRestart(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "history.db")
	writeTestFile(t, root, "main.go", "package main\n")

	service, err := newTestServiceWithHistoryPath(root, dbPath)
	if err != nil {
		t.Fatalf("new changes service failed: %v", err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("close first changes service failed: %v", err)
	}

	writeTestFile(t, root, "main.go", "package main\n\nfunc main() {}\n")

	restarted, err := newTestServiceWithHistoryPath(root, dbPath)
	if err != nil {
		t.Fatalf("restart changes service failed: %v", err)
	}
	defer restarted.Close()

	result, err := restarted.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil {
		t.Fatalf("list changes after restart failed: %v", err)
	}
	if !hasChange(result.Entries, "main.go") {
		t.Fatalf("expected main.go to remain changed after restart: %+v", result.Entries)
	}

	diff, err := restarted.Diff(context.Background(), protocol.ChangeDiffPayload{Path: "main.go"})
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := restarted.Revert(context.Background(), protocol.ChangeRevertPayload{Path: "main.go", ExpectedHash: diff.ExpectedHash})
	if err != nil {
		t.Fatalf("revert after restart failed: %v", err)
	}
	if !reverted.Reverted {
		t.Fatalf("expected reverted=true")
	}
	content, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatalf("read reverted file failed: %v", err)
	}
	if string(content) != "package main\n" {
		t.Fatalf("expected sqlite baseline content, got %q", string(content))
	}
}

func TestHistoryListAndRevertCheckpoint(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "history.db")
	writeTestFile(t, root, "main.go", "package main\n")

	service, err := newTestServiceWithHistoryPath(root, dbPath)
	if err != nil {
		t.Fatalf("new changes service failed: %v", err)
	}
	defer service.Close()

	before, exists, err := history.SnapshotFile(filepath.Join(root, "main.go"), "main.go")
	if err != nil || !exists {
		t.Fatalf("snapshot before failed: exists=%v err=%v", exists, err)
	}
	writeTestFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	after, exists, err := history.SnapshotFile(filepath.Join(root, "main.go"), "main.go")
	if err != nil || !exists {
		t.Fatalf("snapshot after failed: exists=%v err=%v", exists, err)
	}
	checkpointID, err := service.store.SaveCheckpoint(context.Background(), domainhistory.Checkpoint{
		Workspace: service.root,
		Title:     "edit_file main.go",
		Reason:    "test",
	}, []domainhistory.FileChange{{
		Path:       "main.go",
		ChangeType: "modified",
		Before:     &before,
		After:      &after,
	}})
	if err != nil {
		t.Fatalf("save checkpoint failed: %v", err)
	}

	list, err := service.History(context.Background(), protocol.HistoryListPayload{})
	if err != nil {
		t.Fatalf("history list failed: %v", err)
	}
	if len(list.Checkpoints) != 1 || list.Checkpoints[0].ID != checkpointID {
		t.Fatalf("unexpected history list: %+v", list)
	}

	reverted, err := service.RevertCheckpoint(context.Background(), protocol.HistoryRevertPayload{CheckpointID: checkpointID})
	if err != nil {
		t.Fatalf("history revert failed: %v", err)
	}
	if !reverted.Reverted {
		t.Fatalf("expected reverted=true")
	}
	content, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatalf("read reverted file failed: %v", err)
	}
	if string(content) != "package main\n" {
		t.Fatalf("expected checkpoint before content, got %q", string(content))
	}
}

func TestHistoryDiffCheckpoint(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "history.db")
	writeTestFile(t, root, "main.go", "package main\n")

	service, err := newTestServiceWithHistoryPath(root, dbPath)
	if err != nil {
		t.Fatalf("new changes service failed: %v", err)
	}
	defer service.Close()

	before, exists, err := history.SnapshotFile(filepath.Join(root, "main.go"), "main.go")
	if err != nil || !exists {
		t.Fatalf("snapshot before failed: exists=%v err=%v", exists, err)
	}
	writeTestFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	after, exists, err := history.SnapshotFile(filepath.Join(root, "main.go"), "main.go")
	if err != nil || !exists {
		t.Fatalf("snapshot after failed: exists=%v err=%v", exists, err)
	}
	checkpointID, err := service.store.SaveCheckpoint(context.Background(), domainhistory.Checkpoint{
		Workspace: service.root,
		Title:     "edit_file main.go",
		Reason:    "test",
	}, []domainhistory.FileChange{{
		Path:       "main.go",
		ChangeType: "modified",
		Before:     &before,
		After:      &after,
	}})
	if err != nil {
		t.Fatalf("save checkpoint failed: %v", err)
	}

	diff, err := service.HistoryDiff(context.Background(), protocol.HistoryDiffPayload{CheckpointID: checkpointID})
	if err != nil {
		t.Fatalf("history diff failed: %v", err)
	}
	if diff.CheckpointID != checkpointID {
		t.Fatalf("expected checkpoint id %s, got %s", checkpointID, diff.CheckpointID)
	}
	if len(diff.Files) != 1 {
		t.Fatalf("expected one file diff, got %+v", diff.Files)
	}
	if diff.Files[0].Path != "main.go" || diff.Files[0].ChangeType != "modified" {
		t.Fatalf("unexpected file diff metadata: %+v", diff.Files[0])
	}
	if !strings.Contains(diff.Files[0].Diff, "+func main() {}") {
		t.Fatalf("expected checkpoint diff content, got:\n%s", diff.Files[0].Diff)
	}
	if !diff.Files[0].Restorable {
		t.Fatalf("expected checkpoint file to be restorable")
	}
}

func TestDiffRejectsUnsafePath(t *testing.T) {
	service := newTestService(t, t.TempDir())

	if _, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: ".."}); err == nil {
		t.Fatalf("expected path traversal to be rejected")
	}
}

func TestEmptyBaselineSurvivesRestart(t *testing.T) {
	root, dbPath := t.TempDir(), filepath.Join(t.TempDir(), "history.db")
	service, err := newTestServiceWithHistoryPath(root, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "new.txt", "new after empty baseline")
	service, err = newTestServiceWithHistoryPath(root, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	list, err := service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil || !hasChange(list.Entries, "new.txt") {
		t.Fatal("empty baseline was overwritten on restart")
	}
}

func TestCheckpointCreatedBeforeChangesDoesNotSuppressBaseline(t *testing.T) {
	root, dbPath := t.TempDir(), filepath.Join(t.TempDir(), "history.db")
	writeTestFile(t, root, "notes.txt", "baseline")
	store, err := sqlitehistory.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	file, exists, err := history.SnapshotFile(filepath.Join(root, "notes.txt"), "notes.txt")
	if err != nil || !exists {
		t.Fatal("snapshot fixture failed")
	}
	workspace, err := normalizeRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveCheckpoint(context.Background(), domainhistory.Checkpoint{Workspace: workspace}, []domainhistory.FileChange{{Path: file.Path, After: &file}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	service, err := newTestServiceWithHistoryPath(root, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	list, err := service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil || !list.Clean {
		t.Fatal("checkpoint-only workspace prevented initial baseline")
	}
	writeTestFile(t, root, "notes.txt", "after")
	list, err = service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil || len(list.Entries) != 1 || list.Entries[0].Status != "modified" {
		t.Fatal("initial baseline was not established")
	}
}

func TestLegacyEmptyWorkspaceRebuildsBaselineOnce(t *testing.T) {
	root, dbPath := t.TempDir(), filepath.Join(t.TempDir(), "legacy.db")
	workspace, err := normalizeRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqlitehistory.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceBaseline(context.Background(), workspace, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`DROP TABLE workspace_baselines`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "notes.txt", "rebuilt baseline")
	service, err := newTestServiceWithHistoryPath(root, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "notes.txt", "later change")
	service, err = newTestServiceWithHistoryPath(root, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	list, err := service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil || len(list.Entries) != 1 || list.Entries[0].Status != "modified" {
		t.Fatal("legacy empty workspace baseline was rebuilt more than once")
	}
}

func newTestService(t *testing.T, root string) *Service {
	t.Helper()

	service, err := newTestServiceWithHistoryPath(root, filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("new changes service failed: %v", err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close changes service failed: %v", err)
		}
	})
	return service
}

func writeTestFile(t *testing.T, root string, path string, content string) {
	t.Helper()

	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create test parent directory failed: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file failed: %v", err)
	}
}

func findChange(entries []protocol.ChangeEntry, path string) (protocol.ChangeEntry, bool) {
	for _, entry := range entries {
		if entry.Path == path {
			return entry, true
		}
	}
	return protocol.ChangeEntry{}, false
}

func hasChange(entries []protocol.ChangeEntry, path string) bool {
	_, ok := findChange(entries, path)
	return ok
}

func TestListReportsAddedAndRemovedLineCounts(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "mod.txt", "line1\nline2\nline3\n")
	writeTestFile(t, root, "del.txt", "gone1\ngone2\n")

	service := newTestService(t, root)

	writeTestFile(t, root, "mod.txt", "line1\nchanged2\nextra\nline3\n")
	writeTestFile(t, root, "add.txt", "a\nb\nc\n")
	if err := os.Remove(filepath.Join(root, "del.txt")); err != nil {
		t.Fatal(err)
	}

	result, err := service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil {
		t.Fatalf("list changes failed: %v", err)
	}
	if mod, ok := findChange(result.Entries, "mod.txt"); !ok || mod.AddedLines != 2 || mod.RemovedLines != 1 {
		t.Fatalf("unexpected mod.txt counts: %+v", mod)
	}
	if add, ok := findChange(result.Entries, "add.txt"); !ok || add.AddedLines != 3 || add.RemovedLines != 0 {
		t.Fatalf("unexpected add.txt counts: %+v", add)
	}
	if del, ok := findChange(result.Entries, "del.txt"); !ok || del.AddedLines != 0 || del.RemovedLines != 2 {
		t.Fatalf("unexpected del.txt counts: %+v", del)
	}
}
