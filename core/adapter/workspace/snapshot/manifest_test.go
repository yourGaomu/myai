package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainworkspace "myai/core/domain/workspace"
)

func TestSaveManifestKeepsPreviousFileWhenAtomicReplaceFails(t *testing.T) {
	root := t.TempDir()
	original := manifest{
		Version:      1,
		WorkspaceID:  "workspace-1",
		SourceRoot:   filepath.Join(root, "source"),
		SnapshotRoot: filepath.Join(root, "snapshot"),
		Status:       domainworkspace.ChangeSetStatusPending,
		CreatedAt:    time.Now(),
	}
	if err := saveManifest(root, original); err != nil {
		t.Fatal(err)
	}

	expected := errors.New("replace failed")
	updated := original
	updated.Status = domainworkspace.ChangeSetStatusApplied
	err := saveManifestWithReplace(root, updated, func(source string, destination string) error {
		if _, statErr := os.Stat(source); statErr != nil {
			t.Fatalf("expected complete temporary manifest: %v", statErr)
		}
		if _, statErr := os.Stat(destination); statErr != nil {
			t.Fatalf("expected previous manifest to remain until replacement: %v", statErr)
		}
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected replacement error, got %v", err)
	}

	stored, err := loadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domainworkspace.ChangeSetStatusPending {
		t.Fatalf("expected previous manifest status, got %s", stored.Status)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(root, ".manifest-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("expected temporary manifest cleanup, got %#v", temporaryFiles)
	}
}
