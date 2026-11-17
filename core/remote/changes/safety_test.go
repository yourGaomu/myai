package changes

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	domainhistory "myai/core/domain/history"
	historyport "myai/core/port/history"
	"myai/core/remote/protocol"
)

func TestRevertRequiresPreviewAndRejectsLaterChanges(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "notes.txt", "baseline")
	service := newTestService(t, root)
	writeTestFile(t, root, "notes.txt", "previewed")
	preview, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: "notes.txt"})
	if err != nil || preview.ExpectedHash == "" || preview.ExpectedHash == "missing" {
		t.Fatalf("preview hash missing: err=%v", err)
	}
	if _, err := service.Revert(context.Background(), protocol.ChangeRevertPayload{Path: "notes.txt"}); err == nil {
		t.Fatal("revert without expected_hash succeeded")
	}
	writeTestFile(t, root, "notes.txt", "user edit after preview")
	if _, err := service.Revert(context.Background(), protocol.ChangeRevertPayload{Path: "notes.txt", ExpectedHash: preview.ExpectedHash}); err == nil {
		t.Fatal("revert overwrote a later user edit")
	}
	assertFileContent(t, root, "notes.txt", "user edit after preview")
}

func TestCheckpointPreflightLeavesWholeBatchUnchanged(t *testing.T) {
	for _, failure := range []string{"conflict", "unavailable-before", "invalid-before", "symlink", "deleted-recreated", "directory"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			service := newTestService(t, root)
			writeTestFile(t, root, "first.txt", "after one")
			writeTestFile(t, root, "second.txt", "after two")
			first := testSnapshot("first.txt", "after one")
			second := testSnapshot("second.txt", "after two")
			beforeFirst := testSnapshot("first.txt", "before one")
			beforeSecond := testSnapshot("second.txt", "before two")
			var firstAfter = &first
			switch failure {
			case "conflict":
				writeTestFile(t, root, "first.txt", "later user change")
			case "unavailable-before":
				beforeFirst.Available = false
				beforeFirst.Content = nil
			case "invalid-before":
				beforeFirst.Content = []byte("corrupt history")
			case "symlink":
				if err := os.Remove(filepath.Join(root, "first.txt")); err != nil {
					t.Fatal(err)
				}
				createTestSymlink(t, filepath.Join(root, "second.txt"), filepath.Join(root, "first.txt"))
			case "deleted-recreated":
				firstAfter = nil
			case "directory":
				if err := os.Remove(filepath.Join(root, "first.txt")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(root, "first.txt"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			id := saveTestCheckpoint(t, service, []domainhistory.FileChange{
				{Path: "first.txt", Before: &beforeFirst, After: firstAfter},
				{Path: "second.txt", Before: &beforeSecond, After: &second},
			})
			if _, err := service.RevertCheckpoint(context.Background(), protocol.HistoryRevertPayload{CheckpointID: id}); err == nil {
				t.Fatal("unsafe checkpoint revert succeeded")
			}
			assertFileContent(t, root, "second.txt", "after two")
			if failure == "conflict" {
				assertFileContent(t, root, "first.txt", "later user change")
			}
			assertNoRestoreTemps(t, root)
		})
	}
}

func TestCheckpointRecordsInverseAndCanRedo(t *testing.T) {
	root := t.TempDir()
	service := newTestService(t, root)
	writeTestFile(t, root, "modified.txt", "new")
	writeTestFile(t, root, "added.txt", "added")
	before := testSnapshot("modified.txt", "old")
	after := testSnapshot("modified.txt", "new")
	added := testSnapshot("added.txt", "added")
	deleted := testSnapshot("deep/deleted.txt", "restored")
	id := saveTestCheckpoint(t, service, []domainhistory.FileChange{
		{Path: before.Path, Before: &before, After: &after},
		{Path: added.Path, After: &added},
		{Path: deleted.Path, Before: &deleted},
	})
	result, err := service.RevertCheckpoint(context.Background(), protocol.HistoryRevertPayload{CheckpointID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Reverted || result.InverseCheckpointID == "" || result.InverseCheckpointID == id {
		t.Fatalf("missing inverse checkpoint: %#v", result)
	}
	assertFileContent(t, root, "modified.txt", "old")
	assertFileContent(t, root, "deep/deleted.txt", "restored")
	if _, err := os.Lstat(filepath.Join(root, "added.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("added file remains: %v", err)
	}
	if _, err := service.RevertCheckpoint(context.Background(), protocol.HistoryRevertPayload{CheckpointID: result.InverseCheckpointID}); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, root, "modified.txt", "new")
	assertFileContent(t, root, "added.txt", "added")
	if _, err := os.Lstat(filepath.Join(root, "deep/deleted.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("redo did not remove deleted file: %v", err)
	}
	assertNoRestoreTemps(t, root)
}

func TestCheckpointPartialWriteFailureCompensates(t *testing.T) {
	root := t.TempDir()
	service := newTestService(t, root)
	var changes []domainhistory.FileChange
	for _, name := range []string{"one.txt", "two.txt", "three.txt"} {
		writeTestFile(t, root, name, "after")
		before := testSnapshot(name, "before")
		after := testSnapshot(name, "after")
		changes = append(changes, domainhistory.FileChange{Path: name, Before: &before, After: &after})
	}
	id := saveTestCheckpoint(t, service, changes)
	writes := 0
	service.renameFile = func(root *os.Root, from, to string) error {
		if strings.HasPrefix(from, ".myai-revert-after-") {
			writes++
			if writes == 2 {
				return errors.New("injected second replacement failure")
			}
		}
		return root.Rename(from, to)
	}
	if _, err := service.RevertCheckpoint(context.Background(), protocol.HistoryRevertPayload{CheckpointID: id}); err == nil {
		t.Fatal("expected replacement failure")
	}
	if writes != 2 {
		t.Fatalf("expected partial application, got %d attempts", writes)
	}
	for _, name := range []string{"one.txt", "two.txt", "three.txt"} {
		assertFileContent(t, root, name, "after")
	}
	assertNoRestoreTemps(t, root)
}

func TestCheckpointHistoryFailureCompensatesAddedDeletedAndModified(t *testing.T) {
	root := t.TempDir()
	service := newTestService(t, root)
	writeTestFile(t, root, "modified.txt", "after")
	writeTestFile(t, root, "added.txt", "added")
	before := testSnapshot("modified.txt", "before")
	after := testSnapshot("modified.txt", "after")
	added := testSnapshot("added.txt", "added")
	deleted := testSnapshot("missing/deleted.txt", "deleted")
	id := saveTestCheckpoint(t, service, []domainhistory.FileChange{
		{Path: before.Path, Before: &before, After: &after},
		{Path: added.Path, After: &added},
		{Path: deleted.Path, Before: &deleted},
	})
	service.store = failSaveStore{Store: service.store}
	if _, err := service.RevertCheckpoint(context.Background(), protocol.HistoryRevertPayload{CheckpointID: id}); err == nil {
		t.Fatal("expected inverse checkpoint save failure")
	}
	assertFileContent(t, root, "modified.txt", "after")
	assertFileContent(t, root, "added.txt", "added")
	if _, err := os.Lstat(filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("compensation left a created directory: %v", err)
	}
	assertNoRestoreTemps(t, root)
}

func TestChangesRejectRealSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeTestFile(t, outside, "outside.txt", "outside sentinel")
	writeTestFile(t, root, "inside.txt", "inside sentinel")
	createTestSymlink(t, filepath.Join(outside, "outside.txt"), filepath.Join(root, "file-link.txt"))
	createTestSymlink(t, outside, filepath.Join(root, "dir-link"))
	createTestSymlink(t, "inside.txt", filepath.Join(root, "internal-link.txt"))
	service := newTestService(t, root)
	assertRejectedLinkPaths(t, service, []string{"file-link.txt", "dir-link/outside.txt", "dir-link/missing.txt", "internal-link.txt"})
	assertFileContent(t, outside, "outside.txt", "outside sentinel")
	assertFileContent(t, root, "inside.txt", "inside sentinel")
}

func TestChangesRejectRealWindowsJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows junction test")
	}
	root, outside := t.TempDir(), t.TempDir()
	writeTestFile(t, outside, "outside.txt", "outside sentinel")
	junction := filepath.Join(root, "junction")
	cmd := exec.Command("cmd", "/c", "mklink", "/J", junction, outside)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create test junction: %v (%s)", err, output)
	}
	t.Cleanup(func() { _ = os.Remove(junction) })
	info, err := os.Lstat(junction)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0 {
		t.Fatalf("junction mode not recognized: %v", info.Mode())
	}
	service := newTestService(t, root)
	assertRejectedLinkPaths(t, service, []string{"junction/outside.txt", "junction/missing.txt"})
	assertFileContent(t, outside, "outside.txt", "outside sentinel")
}

func TestChangesCannotFollowWorkspaceLinkSwap(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeTestFile(t, root, "sub/notes.txt", "before")
	writeTestFile(t, outside, "notes.txt", "outside sentinel")
	service := newTestService(t, root)
	writeTestFile(t, root, "sub/notes.txt", "after")
	preview, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: "sub/notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "sub"), filepath.Join(root, "sub-original")); err != nil {
		t.Fatal(err)
	}
	createTestSymlink(t, outside, filepath.Join(root, "sub"))
	if _, err := service.Revert(context.Background(), protocol.ChangeRevertPayload{Path: "sub/notes.txt", ExpectedHash: preview.ExpectedHash}); err == nil {
		t.Fatal("revert followed a swapped parent")
	}
	assertFileContent(t, outside, "notes.txt", "outside sentinel")
	assertFileContent(t, root, "sub-original/notes.txt", "after")
}

func assertRejectedLinkPaths(t *testing.T, service *Service, paths []string) {
	t.Helper()
	for _, name := range paths {
		if _, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: name}); err == nil {
			t.Fatalf("diff allowed link path %s", name)
		}
		if _, err := service.Revert(context.Background(), protocol.ChangeRevertPayload{Path: name, ExpectedHash: "missing"}); err == nil {
			t.Fatalf("revert allowed link path %s", name)
		}
	}
	list, err := service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range list.Entries {
		for _, name := range paths {
			if entry.Path == name {
				t.Fatalf("scan followed a link: %s", name)
			}
		}
	}
}

func TestHistoryDiffRejectsLegacySensitiveContentAndUnavailableAddition(t *testing.T) {
	root := t.TempDir()
	service := newTestService(t, root)
	for _, name := range []string{".env.staging", "nested/Application.YAML", `nested\relay.yaml`, "credentials/private.pem", ".git/config"} {
		snapshot := testSnapshot(name, "synthetic legacy fixture")
		service.store = legacyChangesStore{Store: service.store, changes: []domainhistory.StoredFileChange{{
			Path: name, After: &snapshot,
		}}}
		result, err := service.HistoryDiff(context.Background(), protocol.HistoryDiffPayload{CheckpointID: "legacy"})
		if err == nil || len(result.Files) != 0 {
			t.Fatal("legacy protected diff was returned")
		}
		if _, err := service.RevertCheckpoint(context.Background(), protocol.HistoryRevertPayload{CheckpointID: "legacy"}); err == nil {
			t.Fatal("legacy protected checkpoint was reverted")
		}
	}
	unavailable := testSnapshot("safe.txt", "unavailable fixture")
	unavailable.Available = false
	item := storedChangeDiff(domainhistory.StoredFileChange{Path: "safe.txt", After: &unavailable})
	if item.Diff != "" || !item.Binary {
		t.Fatal("unavailable addition produced a diff")
	}
}

func TestLegacySensitiveBaselineIsExcludedAfterUpgrade(t *testing.T) {
	store := &legacyBaselineStore{fakeHistoryStore: fakeHistoryStore{hasBaseline: true}}
	service, err := NewWithStore(t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	list, err := service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil {
		t.Fatal(err)
	}
	if !list.Clean || len(service.baseline) != 0 {
		t.Fatal("protected legacy baseline treated as deleted")
	}
}

func testSnapshot(name, content string) domainhistory.FileSnapshot {
	mode := os.FileMode(0o644)
	if runtime.GOOS == "windows" {
		mode = 0o666
	}
	return domainhistory.FileSnapshot{
		Path: name, Content: []byte(content), Size: int64(len(content)),
		Hash: sha256.Sum256([]byte(content)), Mode: mode, Available: true,
	}
}

func saveTestCheckpoint(t *testing.T, service *Service, changes []domainhistory.FileChange) string {
	t.Helper()
	id, err := service.store.SaveCheckpoint(context.Background(), domainhistory.Checkpoint{Workspace: service.root, Title: "test batch"}, changes)
	if err != nil || id == "" {
		t.Fatalf("save checkpoint: id=%q err=%v", id, err)
	}
	return id
}

func assertFileContent(t *testing.T, root, name, expected string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("unexpected content for %s", name)
	}
}

func assertNoRestoreTemps(t *testing.T, root string) {
	t.Helper()
	if err := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".myai-revert-") {
			t.Fatalf("restore temporary file retained: %s", name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func createTestSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
			t.Skip("Windows account lacks symlink privilege; the real junction test still runs")
		}
		t.Fatalf("create real test symlink: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(link) })
}

type failSaveStore struct{ historyport.Store }

func (failSaveStore) SaveCheckpoint(context.Context, domainhistory.Checkpoint, []domainhistory.FileChange) (string, error) {
	return "", errors.New("injected history write failure")
}

type legacyChangesStore struct {
	historyport.Store
	changes []domainhistory.StoredFileChange
}

func (s legacyChangesStore) LoadCheckpointChanges(context.Context, string, string) ([]domainhistory.StoredFileChange, error) {
	return s.changes, nil
}

type legacyBaselineStore struct{ fakeHistoryStore }

func (*legacyBaselineStore) LoadBaseline(context.Context, string) (map[string]domainhistory.FileSnapshot, error) {
	return map[string]domainhistory.FileSnapshot{".env.staging": testSnapshot(".env.staging", "synthetic legacy fixture")}, nil
}
