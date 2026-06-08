package changes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"myai/core/remote/protocol"
)

func TestListAndDiffGitChanges(t *testing.T) {
	root := t.TempDir()
	runTestGit(t, root, "init")
	runTestGit(t, root, "config", "user.email", "test@example.com")
	runTestGit(t, root, "config", "user.name", "Test User")

	writeTestFile(t, root, "main.go", "package main\n")
	runTestGit(t, root, "add", "main.go")
	runTestGit(t, root, "commit", "-m", "initial")

	writeTestFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	writeTestFile(t, root, "new.txt", "hello\n")

	service := newTestService(t, root)
	result, err := service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil {
		t.Fatalf("list changes failed: %v", err)
	}
	if result.Clean {
		t.Fatalf("expected dirty repository")
	}
	if !hasChange(result.Entries, "main.go") {
		t.Fatalf("expected main.go change: %+v", result.Entries)
	}
	if !hasChange(result.Entries, "new.txt") {
		t.Fatalf("expected new.txt change: %+v", result.Entries)
	}

	diff, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: "main.go"})
	if err != nil {
		t.Fatalf("diff changed file failed: %v", err)
	}
	if !strings.Contains(diff.Diff, "+func main() {}") {
		t.Fatalf("expected main.go diff, got:\n%s", diff.Diff)
	}

	untracked, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: "new.txt"})
	if err != nil {
		t.Fatalf("diff untracked file failed: %v", err)
	}
	if !strings.Contains(untracked.Diff, "new file mode") || !strings.Contains(untracked.Diff, "+hello") {
		t.Fatalf("expected untracked file diff, got:\n%s", untracked.Diff)
	}
}

func TestListRespectsWorkspaceSubdirectory(t *testing.T) {
	root := t.TempDir()
	runTestGit(t, root, "init")
	runTestGit(t, root, "config", "user.email", "test@example.com")
	runTestGit(t, root, "config", "user.name", "Test User")

	writeTestFile(t, root, "app/main.go", "package main\n")
	writeTestFile(t, root, "README.md", "# demo\n")
	runTestGit(t, root, "add", ".")
	runTestGit(t, root, "commit", "-m", "initial")

	writeTestFile(t, root, "app/main.go", "package main\n\nfunc main() {}\n")
	writeTestFile(t, root, "README.md", "# changed\n")

	service := newTestService(t, filepath.Join(root, "app"))
	result, err := service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil {
		t.Fatalf("list changes failed: %v", err)
	}
	if !hasChange(result.Entries, "main.go") {
		t.Fatalf("expected workspace-relative main.go change: %+v", result.Entries)
	}
	if hasChange(result.Entries, "README.md") {
		t.Fatalf("did not expect change outside workspace: %+v", result.Entries)
	}

	diff, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: "main.go"})
	if err != nil {
		t.Fatalf("diff workspace-relative file failed: %v", err)
	}
	if !strings.Contains(diff.Diff, "+func main() {}") {
		t.Fatalf("expected main.go diff, got:\n%s", diff.Diff)
	}
}

func TestListOutsideGitRepository(t *testing.T) {
	service := newTestService(t, t.TempDir())

	result, err := service.List(context.Background(), protocol.ChangesListPayload{})
	if err != nil {
		t.Fatalf("list outside repository failed: %v", err)
	}
	if result.Repository {
		t.Fatalf("expected repository=false")
	}
	if !result.Clean {
		t.Fatalf("outside repository should be treated as clean")
	}
}

func TestDiffRejectsUnsafePath(t *testing.T) {
	root := t.TempDir()
	runTestGit(t, root, "init")
	service := newTestService(t, root)

	if _, err := service.Diff(context.Background(), protocol.ChangeDiffPayload{Path: ".."}); err == nil {
		t.Fatalf("expected path traversal to be rejected")
	}
}

func newTestService(t *testing.T, root string) *Service {
	t.Helper()

	service, err := New(root)
	if err != nil {
		t.Fatalf("new changes service failed: %v", err)
	}
	return service
}

func runTestGit(t *testing.T, root string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}

func writeTestFile(t *testing.T, root string, path string, content string) {
	t.Helper()

	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create test parent directory failed: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file failed: %v", err)
	}
}

func hasChange(entries []protocol.ChangeEntry, path string) bool {
	for _, entry := range entries {
		if entry.Path == path {
			return true
		}
	}
	return false
}
