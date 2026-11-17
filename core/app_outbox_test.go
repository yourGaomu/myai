package core

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistenceScopeBindsDatabaseAndWorkspaceWithoutCredentials(t *testing.T) {
	first, err := persistenceScope("mongodb://test-user:test-password@db.example.test:27017/?authSource=admin", "chat", "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	same, err := persistenceScope("mongodb://other-user:other-password@db.example.test:27017/?retryWrites=true", "chat", "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if first != same || len(first) != 64 {
		t.Fatal("credential rotation must not change the non-secret backend scope")
	}
	for _, changed := range []struct{ uri, database, workspace string }{
		{"mongodb://another.example.test:27017", "chat", "/workspace"},
		{"mongodb://db.example.test:27017", "other", "/workspace"},
		{"mongodb://db.example.test:27017", "chat", "/other"},
	} {
		got, err := persistenceScope(changed.uri, changed.database, changed.workspace)
		if err != nil {
			t.Fatal(err)
		}
		if got == first {
			t.Fatal("a different persistence target reused the same journal scope")
		}
	}
	if strings.Contains(first, "test-user") || strings.Contains(first, "test-password") {
		t.Fatal("journal scope contains credentials")
	}
}

func TestPersistenceJournalIsOutsideToolWorkspace(t *testing.T) {
	workspace, err := canonicalPersistenceWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePersistencePath(workspace, filepath.Join(workspace, ".myai", "outbox.db")); err == nil {
		t.Fatal("journal inside the tool workspace must be rejected")
	}
	if err := validatePersistencePath(workspace, filepath.Join(t.TempDir(), "private", "outbox.db")); err != nil {
		t.Fatal(err)
	}
	if err := validatePersistencePath(workspace, "relative.db"); err == nil {
		t.Fatal("relative journal path was accepted")
	}
}

func TestPersistenceScopeRejectsInvalidTargetWithoutEcho(t *testing.T) {
	for _, uri := range []string{"", "https://db.example.test", "mongodb://%broken-credential@db.example.test"} {
		_, err := persistenceScope(uri, "chat", "/workspace")
		if err == nil {
			t.Fatal("invalid backend identity was accepted")
		}
		if strings.Contains(err.Error(), "credential") || (uri != "" && strings.Contains(err.Error(), uri)) {
			t.Fatal("backend identity error echoed input")
		}
	}
}
