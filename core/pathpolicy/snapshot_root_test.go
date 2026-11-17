package pathpolicy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSnapshotWorkspaceClassifiesItsOwnRelativeContents(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), ".myai", "subagent-snapshots", "job", "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	policy := Policy{WorkspaceRoot: workspace}
	for _, path := range []string{"reports/result.txt", filepath.Join(workspace, "reports", "result.txt")} {
		if policy.RequiresApproval(path) || policy.Sensitive(path) || policy.SkipSnapshot(path) {
			t.Errorf("ordinary child artifact inherited storage protection: %s", path)
		}
		if _, err := policy.Resolve(path); err != nil {
			t.Errorf("ordinary child artifact resolution failed: %v", err)
		}
	}
	for _, relative := range []string{".env.test", "config/demo.yaml", ".git/HEAD", ".myai/state.json", "privatekeys/demo", "plugins/demo/plugin.json"} {
		for _, path := range []string{relative, filepath.Join(workspace, relative)} {
			if !policy.RequiresApproval(path) || !policy.SkipSnapshot(path) {
				t.Errorf("sensitive child content escaped protection: %s", path)
			}
		}
	}
	for _, path := range []string{workspace + string(filepath.Separator) + ".git" + string(filepath.Separator) + ".." + string(filepath.Separator) + "report.txt", ".git/../report.txt"} {
		if !policy.RequiresApproval(path) {
			t.Errorf("protected original path was cleaned before classification: %s", path)
		}
	}
}

func TestWorkspaceRootCannotReclassifyTrustOrGitMetadata(t *testing.T) {
	base := t.TempDir()
	for _, root := range []string{filepath.Join(base, ".git"), filepath.Join(base, ".git", "objects")} {
		if _, err := (Policy{WorkspaceRoot: root}).Resolve("ordinary.txt"); err == nil {
			t.Fatal("git metadata became an ordinary workspace")
		}
	}
	configHome := filepath.Join(base, "user-settings")
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", configHome)
	case "darwin", "ios":
		t.Setenv("HOME", base)
	default:
		t.Setenv("XDG_CONFIG_HOME", configHome)
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(configRoot, "myai")
	if _, err := (Policy{WorkspaceRoot: root}).Resolve("ordinary.txt"); err == nil {
		t.Fatal("user approval root became an ordinary workspace")
	}
}
