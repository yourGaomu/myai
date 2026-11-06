package trust

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"myai/core/processpolicy"
)

type trustProcessHelper struct {
	process *processpolicy.Process
	stdin   io.WriteCloser
	lines   <-chan string
	stderr  *bytes.Buffer
}

func startTrustProcessHelper(t *testing.T, ctx context.Context, path, workspace, action, declaration string) trustProcessHelper {
	t.Helper()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTrustStoreProcessHelper$", "--", "trust-lock-helper", path, workspace, action, declaration)
	command.Dir = t.TempDir()
	command.Env = processpolicy.Environment(nil)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	process, err := processpolicy.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = process.Kill() })
	lines := make(chan string, 4)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	return trustProcessHelper{process, stdin, lines, stderr}
}

func expectTrustProcessLine(t *testing.T, ctx context.Context, helper trustProcessHelper, expected string) {
	t.Helper()
	select {
	case line := <-helper.lines:
		if line != expected {
			t.Fatalf("helper expected %q, got %q", expected, line)
		}
	case <-ctx.Done():
		t.Fatalf("helper did not reach %q: %v", expected, ctx.Err())
	}
}

func TestStoreConcurrentProcessesDoNotLoseApprovalOrResurrectRevocation(t *testing.T) {
	workspace, userDirectory := t.TempDir(), t.TempDir()
	store := NewStore(filepath.Join(userDirectory, "trust.json"))
	first := requestForTest(t, workspace, map[string]string{"command": "dummy-a"})
	second := requestForTest(t, workspace, map[string]string{"command": "dummy-b"})
	unchanged := requestForTest(t, workspace, map[string]string{"command": "dummy-c"})
	if err := store.Approve(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Approve(unchanged); err != nil {
		t.Fatal(err)
	}
	lockInfo, err := os.Stat(store.Path() + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := store.lock(first)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() { once.Do(unlock) }
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	revoker := startTrustProcessHelper(t, ctx, store.Path(), workspace, "revoke", "dummy-a")
	approver := startTrustProcessHelper(t, ctx, store.Path(), workspace, "approve", "dummy-b")
	for _, helper := range []trustProcessHelper{revoker, approver} {
		expectTrustProcessLine(t, ctx, helper, "ready")
	}
	// Both real CLI-like processes leave the same barrier while a third owner
	// holds the stable lock. Neither may finish before that owner releases it.
	for _, helper := range []trustProcessHelper{revoker, approver} {
		if _, err := io.WriteString(helper.stdin, "go\n"); err != nil {
			t.Fatal(err)
		}
	}
	for _, helper := range []trustProcessHelper{revoker, approver} {
		expectTrustProcessLine(t, ctx, helper, "attempting")
	}
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case line := <-revoker.lines:
		t.Fatalf("revoke bypassed another process's store lock: %q", line)
	case line := <-approver.lines:
		t.Fatalf("approve bypassed another process's store lock: %q", line)
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	release()
	for _, helper := range []trustProcessHelper{revoker, approver} {
		expectTrustProcessLine(t, ctx, helper, "done")
		if err := helper.process.Wait(); err != nil {
			t.Fatalf("helper failed: %v %s", err, helper.stderr.String())
		}
	}
	if approved, err := store.Trusted(first); err != nil || approved {
		t.Fatalf("revoked A was resurrected: approved=%v err=%v", approved, err)
	}
	if approved, err := store.Trusted(second); err != nil || !approved {
		t.Fatalf("approval B was lost: approved=%v err=%v", approved, err)
	}
	if approved, err := store.Trusted(unchanged); err != nil || !approved {
		t.Fatalf("unrelated approval C was lost: approved=%v err=%v", approved, err)
	}
	currentLock, err := os.Stat(store.Path() + ".lock")
	if err != nil || !os.SameFile(lockInfo, currentLock) {
		t.Fatal("store replacement changed the lock file")
	}
}

func TestTrustStoreProcessHelper(t *testing.T) {
	var args []string
	for index, arg := range os.Args {
		if arg == "trust-lock-helper" {
			args = os.Args[index+1:]
			break
		}
	}
	if len(args) == 0 {
		return
	}
	if len(args) != 4 {
		t.Fatal("invalid trust helper invocation")
	}
	request := requestForTest(t, args[1], map[string]string{"command": args[3]})
	_, _ = fmt.Fprintln(os.Stdout, "ready")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() || scanner.Text() != "go" {
		t.Fatal("trust helper barrier was not released")
	}
	_, _ = fmt.Fprintln(os.Stdout, "attempting")
	store := NewStore(args[0])
	var err error
	if args[2] == "approve" {
		err = store.Approve(request)
	} else if args[2] == "revoke" {
		err = store.Revoke(request)
	} else {
		t.Fatal("invalid trust helper action")
	}
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintln(os.Stdout, "done")
	os.Exit(0)
}

func TestStoreRejectsUnsafeLockFile(t *testing.T) {
	for _, test := range []string{"directory", "hardlink", "symlink", "permissions"} {
		t.Run(test, func(t *testing.T) {
			workspace, userDirectory := t.TempDir(), t.TempDir()
			store := NewStore(filepath.Join(userDirectory, "trust.json"))
			request := requestForTest(t, workspace, map[string]string{"command": "dummy"})
			lockPath := store.Path() + ".lock"
			switch test {
			case "directory":
				if err := os.Mkdir(lockPath, 0700); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				target := filepath.Join(workspace, "linked-lock")
				if err := os.WriteFile(target, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, lockPath); err != nil {
					t.Skip("hard links unavailable")
				}
			case "symlink":
				target := filepath.Join(workspace, "linked-lock")
				if err := os.WriteFile(target, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, lockPath); err != nil {
					t.Skip("symlinks unavailable")
				}
			case "permissions":
				if runtime.GOOS == "windows" {
					t.Skip("POSIX permission bits; Windows validates ACLs")
				}
				if err := os.WriteFile(lockPath, nil, 0666); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(lockPath, 0666); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Approve(request); err == nil {
				t.Fatal("unsafe lock file accepted")
			}
		})
	}
}
