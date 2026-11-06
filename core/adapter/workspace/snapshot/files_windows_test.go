package snapshot

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSnapshotWindowsJunctionCannotEscape(t *testing.T) {
	source, target, outside := t.TempDir(), t.TempDir(), t.TempDir()
	writeTestFile(t, source, "ordinary.txt", "ordinary\n")
	writeTestFile(t, outside, "sentinel.txt", "outside sentinel\n")
	junction := filepath.Join(source, "alias")
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, outside).CombinedOutput(); err != nil {
		t.Skipf("junction creation unavailable: %v (%s)", err, output)
	}
	states, err := copyWorkspace(context.Background(), source, target)
	if err != nil || len(states) != 1 || states[0].Path != "ordinary.txt" {
		t.Fatalf("copy traversed a Windows junction: %#v, %v", states, err)
	}
	scanned, err := scanWorkspace(context.Background(), source)
	if err != nil || len(scanned) != 1 {
		t.Fatalf("scan traversed a Windows junction: %#v, %v", scanned, err)
	}
	if _, err := safeJoin(source, "alias/new.txt"); err == nil {
		t.Fatal("safeJoin accepted a Windows junction component")
	}
	if _, _, err := inspectWorkspaceFile(source, "alias/sentinel.txt"); err == nil {
		t.Fatal("inspection followed a Windows junction")
	}

	destination := t.TempDir()
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(destination, "docs"), outside).CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	writeTestFile(t, source, "docs/new.txt", "must stay inside\n")
	if _, err := copyWorkspace(context.Background(), source, destination); err == nil {
		t.Fatal("copy accepted a destination Windows junction")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("copy created a file outside the snapshot: %v", err)
	}
	assertFileContent(t, filepath.Join(outside, "sentinel.txt"), "outside sentinel\n")
}
