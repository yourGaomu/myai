package grpcprocessor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The test executable doubles as the Python executable. Its first argument is
// the configured Script, so TestMain handles just this private marker and leaves
// normal package tests untouched. No Python installation or service is needed.
func TestMain(m *testing.M) {
	if len(os.Args) >= 3 && filepath.Base(os.Args[1]) == "zz-document-supervisor-helper" {
		runSupervisorHelper()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runSupervisorHelper() {
	directory := ""
	for i, argument := range os.Args {
		if argument == "--temp-dir" && i+1 < len(os.Args) {
			directory = os.Args[i+1]
		}
	}
	if directory == "" {
		os.Exit(10)
	}
	if os.Args[2] == "leaf" {
		_ = os.WriteFile(filepath.Join(directory, "leaf.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
		time.Sleep(15 * time.Second)
		return
	}
	_, secretPresent := os.LookupEnv("ZZ_TEST_SECRET")
	token := os.Getenv("MYAI_DOCUMENT_PROCESSOR_TOKEN")
	if secretPresent || token == "" {
		os.Exit(11)
	}
	// Record only the assertion outcome, never environment values.
	_ = os.WriteFile(filepath.Join(directory, "environment-ok"), []byte("ok"), 0o600)
	child := exec.Command(os.Args[0], os.Args[1], "leaf", "--temp-dir", directory)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Run(); err != nil {
		os.Exit(12)
	}
}

func TestSupervisorStartupTimeoutAndCancellationCleanTree(t *testing.T) {
	t.Setenv("ZZ_TEST_SECRET", "dummy-service-secret")
	for _, mode := range []string{"timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			config := Config{
				PythonExecutable: os.Args[0], Script: filepath.Join(directory, "zz-document-supervisor-helper"),
				TempDir: directory, Transport: TransportTCP, StartupTimeout: 2 * time.Second,
			}.Normalize()
			supervisor := newWorkerSupervisor(config)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := supervisor.Start(ctx, 0)
				result <- err
			}()
			pid := awaitSupervisorPID(t, filepath.Join(directory, "leaf.pid"))
			if _, err := os.Stat(filepath.Join(directory, "environment-ok")); err != nil {
				t.Fatal("document worker environment isolation failed")
			}
			started := time.Now()
			if mode == "cancel" {
				cancel()
			}
			select {
			case err := <-result:
				expected := context.DeadlineExceeded
				if mode == "cancel" {
					expected = context.Canceled
				}
				if !errors.Is(err, expected) {
					t.Fatalf("expected startup context error, got %v", err)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("supervisor startup cleanup exceeded return bound")
			}
			assertSupervisorStopped(t, pid)
			t.Logf("worker startup %s returned in %s with descendant stopped", mode, time.Since(started))
		})
	}
}

func awaitSupervisorPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("supervisor helper did not report descendant PID")
	return 0
}
