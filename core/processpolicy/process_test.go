package processpolicy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func helperCommand(ctx context.Context, mode, directory string) *exec.Cmd {
	return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessHelper$", "--", "processpolicy-helper", mode, directory)
}

func TestProcessTimeoutAndCancellationKillGrandchildren(t *testing.T) {
	for _, mode := range []string{"timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cmd := helperCommand(ctx, "parent", directory)
			// Non-file writers create Go copy goroutines. A leaked grandchild
			// holding either pipe must not keep Wait alive after cancellation.
			cmd.Stdout, cmd.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
			process, err := Start(cmd)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = process.Kill(); _ = process.Wait() })
			child := awaitPID(t, filepath.Join(directory, "child.pid"))
			grandchild := awaitPID(t, filepath.Join(directory, "grandchild.pid"))
			started := time.Now()
			if mode == "cancel" {
				cancel()
			} else {
				<-ctx.Done()
				started = time.Now()
			}
			if err := process.Wait(); err == nil {
				t.Fatal("canceled command unexpectedly succeeded")
			}
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Fatalf("cancellation exceeded return bound: %s", elapsed)
			}
			if cmd.WaitDelay != DefaultWaitDelay {
				t.Fatal("missing bounded pipe wait")
			}
			assertStopped(t, child)
			assertStopped(t, grandchild)
			t.Logf("%s returned after cancellation in %s; child and grandchild stopped", mode, time.Since(started))
		})
	}
}

func TestProcessRootExitCleansDescendantsAndBoundsPipes(t *testing.T) {
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := helperCommand(ctx, "orphan", directory)
	cmd.Stdout, cmd.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
	process, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Kill(); _ = process.Wait() })
	child := awaitPID(t, filepath.Join(directory, "child.pid"))
	grandchild := awaitPID(t, filepath.Join(directory, "grandchild.pid"))
	started := time.Now()
	err = process.Wait()
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("unexpected root exit error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("inherited pipes exceeded return bound: %s", elapsed)
	}
	assertStopped(t, child)
	assertStopped(t, grandchild)
	if !errors.Is(process.Kill(), os.ErrProcessDone) {
		t.Fatal("Kill after Wait should not touch a reused PID")
	}
}

func TestProcessMinimalEnvironment(t *testing.T) {
	t.Setenv("ZZ_TEST_SECRET", "dummy-service-secret")
	for _, explicit := range []bool{false, true} {
		cmd := helperCommand(context.Background(), "environment", "unused")
		if explicit {
			cmd.Env = Environment(map[string]string{"ZZ_TEST_SECRET": "explicit-dummy-secret"})
			cmd.Args = append(cmd.Args, "explicit")
		}
		cmd.Stdout, cmd.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		if err := Run(cmd); err != nil {
			t.Fatalf("environment boundary failed (explicit=%v): %v", explicit, err)
		}
	}
}

func TestProcessHelper(t *testing.T) {
	index := -1
	for i, value := range os.Args {
		if value == "processpolicy-helper" {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	args := os.Args[index+1:]
	if len(args) < 2 {
		os.Exit(2)
	}
	mode, directory := args[0], args[1]
	if mode == "environment" {
		value, present := os.LookupEnv("ZZ_TEST_SECRET")
		if len(args) > 2 {
			if !present || value != "explicit-dummy-secret" {
				os.Exit(3)
			}
		} else if present {
			os.Exit(4)
		}
		os.Exit(0)
	}
	if mode == "grandchild" {
		_ = os.WriteFile(filepath.Join(directory, "grandchild.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
		time.Sleep(15 * time.Second) // watchdog if the test process crashes
		os.Exit(0)
	}
	next := "child"
	if mode == "child" {
		_ = os.WriteFile(filepath.Join(directory, "child.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
		next = "grandchild"
	}
	cmd := helperCommand(context.Background(), next, directory)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		os.Exit(5)
	}
	if mode == "orphan" {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(directory, "grandchild.pid")); err == nil {
				os.Exit(0)
			}
			time.Sleep(time.Millisecond)
		}
		os.Exit(6)
	}
	_ = cmd.Wait()
	os.Exit(0)
}

func awaitPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("helper process did not report its PID")
	return 0
}

func assertStopped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error(fmt.Sprintf("descendant %d remained alive after Wait", pid))
}
