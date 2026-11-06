package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	domainworkspace "myai/core/domain/workspace"
	workspaceport "myai/core/port/workspace"
)

func TestCopyAndScanWorkspaceFilterSharedPolicy(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	allowed := []string{"docs/report.txt", "src/config.go", "src/configuration.go", "src/environment.go", "src/main.go"}
	excluded := []string{
		".env", "nested/.ENV.staging", "application.yaml", "relay.yml", "config/service.go",
		"nested/CONFIG/settings.txt", "privatekeys/signing.txt", "keys/privatekey.pem", "keys/id_ed25519",
		".myai/history.db", "nested/.git/HEAD", "node_modules/package/index.js", "plugins/demo/main.go",
	}
	for _, path := range append(append([]string{}, allowed...), excluded...) {
		writeTestFile(t, source, path, "temporary fixture for "+path+"\n")
	}
	states, err := copyWorkspace(context.Background(), source, target)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(states))
	for _, state := range states {
		got = append(got, state.Path)
		hash := sha256.Sum256([]byte("temporary fixture for " + state.Path + "\n"))
		if state.Hash != hex.EncodeToString(hash[:]) {
			t.Fatalf("copied file hash does not match contents: %#v", state)
		}
		assertFileContent(t, filepath.Join(target, filepath.FromSlash(state.Path)), "temporary fixture for "+state.Path+"\n")
	}
	if !reflect.DeepEqual(got, allowed) {
		t.Fatalf("copied paths = %#v, want %#v", got, allowed)
	}
	for _, path := range excluded {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(path))); !os.IsNotExist(err) {
			t.Fatalf("excluded fixture reached the snapshot: %s, %v", path, err)
		}
	}
	// Scan the original tree too, rather than only the already-filtered copy.
	for _, root := range []string{source, target} {
		scanned, err := scanWorkspace(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		paths := make([]string, 0, len(scanned))
		for path := range scanned {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		if !reflect.DeepEqual(paths, allowed) {
			t.Fatalf("scanned paths = %#v, want %#v", paths, allowed)
		}
	}
}

func TestSnapshotLegacySensitiveBaselineDoesNotDeleteSourceFiles(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, source, "main.go", "before\n")
	for _, path := range []string{".env", "config/service.json", "keys/privatekey.pem", "application.yaml"} {
		writeTestFile(t, source, path, "local temporary sentinel\n")
	}
	manager, err := New(t.TempDir(), fakeHistoryFactory{store: &fakeHistoryStore{}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{
		WorkspaceID: "legacy-sensitive-baseline", Mode: domainworkspace.IsolationModeSnapshot, SourceRoot: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	jobRoot := filepath.Dir(prepared.Reference.Root)
	value, err := loadManifest(jobRoot)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a persisted manifest from before the stricter shared policy.
	for _, path := range []string{".env", "config/service.json", "keys/privatekey.pem", "application.yaml", "../outside.txt"} {
		value.BaselineFiles = append(value.BaselineFiles, fileState{Path: path, Hash: "old-hash", Size: 12})
	}
	if err := saveManifest(jobRoot, value); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, prepared.Reference.Root, "main.go", "after\n")
	collected, err := manager.Collect(context.Background(), workspaceport.CollectRequest{Reference: prepared.Reference})
	if err != nil || len(collected.ChangeSet.Files) != 1 || collected.ChangeSet.Files[0].Path != "main.go" {
		t.Fatalf("legacy manifest produced an excluded deletion: %#v, %v", collected.ChangeSet.Files, err)
	}
	if _, err := manager.Apply(context.Background(), workspaceport.ApplyRequest{Reference: prepared.Reference}); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(source, "main.go"), "after\n")
	for _, path := range []string{".env", "config/service.json", "keys/privatekey.pem", "application.yaml"} {
		assertFileContent(t, filepath.Join(source, filepath.FromSlash(path)), "local temporary sentinel\n")
	}
}

func TestSnapshotRejectsTraversalAndAmbiguousPaths(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"../outside.txt", "a/../../outside.txt", "/outside.txt", `a\outside.txt`, "file.txt:stream", "dir./file.txt", "dir /file.txt", "", "."} {
		if _, err := safeJoin(root, path); err == nil {
			t.Errorf("unsafe path accepted: %q", path)
		}
	}
}

func TestSnapshotSkipsSymlinksAndRejectsLinkedComponents(t *testing.T) {
	source, target, outside := t.TempDir(), t.TempDir(), t.TempDir()
	writeTestFile(t, source, "ordinary.txt", "ordinary\n")
	writeTestFile(t, outside, "outside.txt", "outside sentinel\n")
	if err := os.Symlink(outside, filepath.Join(source, "alias")); err != nil {
		t.Skipf("directory symlink creation unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.txt"), filepath.Join(source, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	states, err := copyWorkspace(context.Background(), source, target)
	if err != nil || len(states) != 1 || states[0].Path != "ordinary.txt" {
		t.Fatalf("symlink was copied: %#v, %v", states, err)
	}
	scanned, err := scanWorkspace(context.Background(), source)
	if err != nil || len(scanned) != 1 {
		t.Fatalf("symlink was scanned: %#v, %v", scanned, err)
	}
	if _, _, err := inspectWorkspaceFile(source, "alias/outside.txt"); err == nil {
		t.Fatal("inspection followed an escaping directory link")
	}
	if _, err := safeJoin(source, "alias/new.txt"); err == nil {
		t.Fatal("safeJoin accepted a linked ancestor")
	}
	assertFileContent(t, filepath.Join(outside, "outside.txt"), "outside sentinel\n")
}

func TestSnapshotCopyRejectsDestinationLink(t *testing.T) {
	source, target, outside := t.TempDir(), t.TempDir(), t.TempDir()
	writeTestFile(t, source, "docs/report.txt", "report\n")
	if err := os.Symlink(outside, filepath.Join(target, "docs")); err != nil {
		t.Skipf("directory symlink creation unavailable: %v", err)
	}
	if _, err := copyWorkspace(context.Background(), source, target); err == nil {
		t.Fatal("copy accepted a linked destination component")
	}
	if _, err := os.Stat(filepath.Join(outside, "report.txt")); !os.IsNotExist(err) {
		t.Fatalf("copy wrote outside target: %v", err)
	}
}
