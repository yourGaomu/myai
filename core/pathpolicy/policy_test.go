package pathpolicy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassification(t *testing.T) {
	for _, path := range []string{
		".env", "nested/.env.staging", `nested\.ENV.test`, "application.yaml", "relay.yml", "config/service.json",
		"privatekeys/demo", "keys/id_ed25519", "demo.pem", "CONFIG/extra.txt", ".ssh/known_hosts", ".env.local:stream",
	} {
		if !IsSensitive(path) || !IsProtected(path) || !ShouldSkipSnapshot(path) {
			t.Errorf("sensitive path was allowed: %q", path)
		}
	}
	for _, path := range []string{".git/config", ".myai/session.db", "plugins/demo/main.go", "skills/demo/SKILL.md", "AGENTS.md", `.GIT\HEAD`} {
		if !IsProtected(path) || !ShouldSkipSnapshot(path) {
			t.Errorf("protected path was allowed: %q", path)
		}
	}
	for _, path := range []string{"src/main.go", "docs/report.txt", "configuration.go", "environment.go", "monkey.txt", "id_ed25519.pub"} {
		if IsSensitive(path) || IsProtected(path) || ShouldSkipSnapshot(path) {
			t.Errorf("ordinary path was rejected: %q", path)
		}
	}
	if !ShouldSkipDirectory("src/node_modules/demo") || ShouldSkipDirectory("docs") || ShouldSkipDirectory("report.pem") {
		t.Fatal("directory classification confused files with directories")
	}
}

func TestResolveAndProtectedRoots(t *testing.T) {
	workspace := t.TempDir()
	policy := Policy{WorkspaceRoot: workspace, ProtectedRoots: []string{"extensions"}}
	resolved, err := policy.Resolve("src/new.go")
	if err != nil || resolved.Relative != "src/new.go" || !filepath.IsAbs(resolved.Abs) {
		t.Fatalf("resolve ordinary new file = %#v, %v", resolved, err)
	}
	if policy.RequiresApproval("src/new.go") || policy.Sensitive("src/new.go") || policy.SkipSnapshot("src/new.go") {
		t.Fatal("ordinary workspace file requires approval")
	}
	if !policy.RequiresApproval("extensions/demo.go") || !policy.SkipSnapshot("extensions/demo.go") {
		t.Fatal("custom executable declaration root was not protected")
	}
	for _, path := range []string{"../outside.txt", `..\outside.txt`, "report.txt:stream", ".git./HEAD", "docs /file.txt"} {
		if _, err := policy.Resolve(path); err == nil || !policy.RequiresApproval(path) {
			t.Errorf("ambiguous or escaping path was accepted: %q", path)
		}
	}
}

func TestResolveRejectsSymlinkEscapeAndProtectsAliases(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "outside")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	policy := Policy{WorkspaceRoot: workspace}
	if _, err := policy.Resolve("outside/new.txt"); err == nil {
		t.Fatal("nonexistent file under escaping symlink was accepted")
	}
	if err := os.Mkdir(filepath.Join(workspace, "privatekeys"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workspace, "privatekeys"), filepath.Join(workspace, "reports")); err != nil {
		t.Fatal(err)
	}
	if !policy.Sensitive("reports/demo.txt") || !policy.RequiresApproval("reports/demo.txt") {
		t.Fatal("sensitive symlink target was accepted")
	}
	if err := os.Mkdir(filepath.Join(workspace, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workspace, "docs"), filepath.Join(workspace, ".git")); err != nil {
		t.Fatal(err)
	}
	if !policy.RequiresApproval(".git/report.txt") {
		t.Fatal("protected lexical path pointing to ordinary target was accepted")
	}
}
