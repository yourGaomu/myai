package opensandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	domainsandbox "myai/core/domain/sandbox"
)

func TestSynchronizationWindowsJunctionCannotEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeWorkspaceFile(t, outside, "sentinel.txt", "outside sentinel\n")
	junction := filepath.Join(root, "alias")
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, outside).CombinedOutput(); err != nil {
		t.Skipf("junction creation unavailable: %v (%s)", err, output)
	}
	scanned, err := scanLocal(context.Background(), root)
	if err != nil || len(scanned) != 0 {
		t.Fatalf("scan traversed a Windows junction: %#v, %v", scanned, err)
	}
	remote := &fakeRemoteWorkspace{id: "sandbox", files: make(map[string]domainsandbox.File)}
	if err := uploadChanged(context.Background(), remote, root, nil, map[string]fileState{"alias/sentinel.txt": {Path: "alias/sentinel.txt"}}); err == nil {
		t.Fatal("upload followed a Windows junction")
	}
	if _, err := safeLocalPath(root, "alias/sentinel.txt"); err == nil {
		t.Fatal("safeLocalPath accepted a Windows junction")
	}
	if err := writeLocalFile(root, fileState{Path: "alias/new.txt", Mode: 0o600}, []byte("must stay inside")); err == nil {
		t.Fatal("write followed a Windows junction")
	}
	if err := removeLocalFile(root, "alias/sentinel.txt"); err == nil {
		t.Fatal("delete followed a Windows junction")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("sync wrote outside workspace: %v", err)
	}
	assertWorkspaceFile(t, outside, "sentinel.txt", "outside sentinel\n")
}
