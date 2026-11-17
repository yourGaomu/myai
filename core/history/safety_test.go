package history

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	domainhistory "myai/core/domain/history"
)

func TestExplicitRecordersDoNotReadOrRecordProtectedPaths(t *testing.T) {
	root := t.TempDir()
	store := &fakeHistoryStore{}
	recorder, err := NewRecorderWithStore(root, store)
	if err != nil {
		t.Fatal(err)
	}
	task := NewTaskRecorderWithStore(RecordCommand{}, store)
	defer task.Close()
	workspaceRecorder, err := task.WorkspaceRecorder(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env.staging", "nested/Application.YAML", "nested/relay.yaml", "credentials/private.pem", ".git/config", "AGENTS.md"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("synthetic history fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
		before := historyTestSnapshot(name)
		for _, r := range []interface {
			SnapshotPath(string) (*domainhistory.FileSnapshot, error)
			RecordFileChange(context.Context, string, *domainhistory.FileSnapshot, RecordCommand) (string, error)
		}{recorder, workspaceRecorder} {
			snapshot, err := r.SnapshotPath(path)
			if err != nil || snapshot != nil {
				t.Fatal("explicit SnapshotPath returned a protected snapshot")
			}
			id, err := r.RecordFileChange(context.Background(), path, &before, RecordCommand{})
			if err != nil || id != "" {
				t.Fatal("explicit RecordFileChange accepted a protected path")
			}
		}
		if _, exists, err := SnapshotFile(path, name); err != nil || exists {
			t.Fatal("compatibility SnapshotFile returned protected content")
		}
	}
	ids, err := task.Save(context.Background())
	if err != nil || len(ids) != 0 || len(store.changes) != 0 {
		t.Fatal("protected content entered history")
	}
}

func TestWorkspaceManifestCannotReintroduceProtectedSnapshots(t *testing.T) {
	root := t.TempDir()
	store := &fakeHistoryStore{}
	task := NewTaskRecorderWithStore(RecordCommand{}, store)
	defer task.Close()
	recorder, err := task.WorkspaceRecorder(root)
	if err != nil {
		t.Fatal(err)
	}
	before := make(map[string]domainhistory.FileSnapshot)
	for _, name := range []string{".env.staging", "nested/relay.yaml", ".git/config", "ordinary.txt"} {
		snapshot := historyTestSnapshot(name)
		if name == "ordinary.txt" {
			snapshot.Path = "nested/Application.YAML"
		}
		before[name] = snapshot
	}
	count, err := recorder.RecordWorkspaceChanges(context.Background(), before, RecordCommand{})
	if err != nil || count != 0 {
		t.Fatalf("protected manifest became deletions: count=%d err=%v", count, err)
	}
	ids, err := task.Save(context.Background())
	if err != nil || len(ids) != 0 || len(store.changes) != 0 {
		t.Fatal("protected manifest entered history")
	}
}

func TestHistoryRejectsRealSymlinkAncestors(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeHistoryFixture(t, outside, "notes.txt")
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
			t.Skip("Windows account lacks symlink privilege; junction coverage runs separately")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(link) })
	assertHistoryRejectsLink(t, root, "link/notes.txt")
}

func TestHistoryRejectsRealWindowsJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows junction test")
	}
	root, outside := t.TempDir(), t.TempDir()
	writeHistoryFixture(t, outside, "notes.txt")
	link := filepath.Join(root, "link")
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Fatalf("create real junction: %v (%s)", err, output)
	}
	t.Cleanup(func() { _ = os.Remove(link) })
	assertHistoryRejectsLink(t, root, "link/notes.txt")
}

func assertHistoryRejectsLink(t *testing.T, root, name string) {
	t.Helper()
	recorder, err := NewRecorderWithStore(root, &fakeHistoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.SnapshotPath(name); err == nil {
		t.Fatal("recorder followed a linked ancestor")
	}
	if _, err := recorder.RecordFileChange(context.Background(), name, nil, RecordCommand{}); err == nil {
		t.Fatal("recorder recorded through a linked ancestor")
	}
	if _, _, err := SnapshotFile(filepath.Join(root, name), name); err == nil {
		t.Fatal("compatibility snapshot followed a linked ancestor")
	}
	result, err := snapshotWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatal("workspace snapshot followed a link")
	}
}

func TestWorkspaceSnapshotKeepsOrdinaryFilesAndSkipsProtectedFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"ordinary.txt", "nested/source.go", ".env.staging", "nested/relay.yaml", "config/ordinary.txt", "AGENTS.md"} {
		writeHistoryFixture(t, root, name)
	}
	result, err := snapshotWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result["ordinary.txt"].Path == "" || result["nested/source.go"].Path == "" {
		t.Fatal("workspace snapshot filtering is incorrect")
	}
}

func writeHistoryFixture(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func historyTestSnapshot(name string) domainhistory.FileSnapshot {
	return domainhistory.FileSnapshot{Path: name, Content: []byte("before"), Size: 6, Hash: sha256.Sum256([]byte("before")), Mode: 0o644, Available: true}
}
