package pathpolicy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestActualUserConfigurationDirectoryIsProtected(t *testing.T) {
	workspace := t.TempDir()
	configHome := filepath.Join(workspace, "user-settings")
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", configHome)
	case "darwin", "ios":
		t.Setenv("HOME", workspace)
		configHome = filepath.Join(workspace, "Library", "Application Support")
	default:
		t.Setenv("XDG_CONFIG_HOME", configHome)
	}
	root, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	policy := Policy{WorkspaceRoot: workspace}
	for _, path := range []string{filepath.Join(root, "myai"), filepath.Join(root, "myai", "trust.json"), filepath.Join(root, "myai", "preferences.json")} {
		if !IsSensitive(path) || !IsProtected(path) || !policy.RequiresApproval(path) || !policy.SkipSnapshot(path) {
			t.Errorf("user approval storage was unprotected: %q", path)
		}
	}
	if policy.RequiresApproval("src/main.go") {
		t.Fatal("ordinary workspace path was classified as user config")
	}
	for _, path := range []string{`AppData\Roaming\myai\preferences.json`, ".config/myai/preferences.json", "myai/trust.json"} {
		if !IsSensitive(path) {
			t.Errorf("conventional approval storage was unprotected: %q", path)
		}
	}
}
