package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	domainexecution "myai/core/domain/execution"
)

func TestExecutorReportsHostExecutionWithoutIsolation(t *testing.T) {
	executor, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Run(context.Background(), domainexecution.RunRequest{Command: "go version"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.ExecutionEnvironment != "local-host" || result.Isolated {
		t.Fatalf("unexpected local execution metadata: %#v", result)
	}
}

func TestExecutorProcessEnvironmentBoundary(t *testing.T) {
	t.Setenv("ZZ_TEST_SECRET", "dummy-service-secret")
	executor, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Run(context.Background(), domainexecution.RunRequest{Command: localHelperCommand("environment"), Timeout: time.Minute})
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("shell inherited a service variable or failed: %v, exit=%d", err, result.ExitCode)
	}
}

// readyCancellationContext triggers a caller cancellation or deadline only after
// the helper reports readiness. Startup has its own generous budget; the strict
// post-cancellation bound must not depend on concurrent PowerShell startup load.
// processpolicy separately exercises cancellation from a real context timer.
type readyCancellationContext struct {
	context.Context
	done  chan struct{}
	cause error
	once  sync.Once
}

func (ctx *readyCancellationContext) Done() <-chan struct{} { return ctx.done }
func (ctx *readyCancellationContext) Err() error {
	select {
	case <-ctx.done:
		return ctx.cause
	default:
		return nil
	}
}
func (ctx *readyCancellationContext) cancel() { ctx.once.Do(func() { close(ctx.done) }) }

func TestExecutorTimeoutAndCancellationCleanDescendants(t *testing.T) {
	for _, mode := range []string{"timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			executor, err := New(directory)
			if err != nil {
				t.Fatal(err)
			}
			cause := context.Canceled
			if mode == "timeout" {
				cause = context.DeadlineExceeded
			}
			ctx := &readyCancellationContext{Context: context.Background(), done: make(chan struct{}), cause: cause}
			response := make(chan struct {
				result domainexecution.RunResult
				err    error
			}, 1)
			completed := make(chan struct{})
			go func() {
				defer close(completed)
				result, err := executor.Run(ctx, domainexecution.RunRequest{
					Command: localHelperCommand("tree", directory), Timeout: time.Minute,
				})
				response <- struct {
					result domainexecution.RunResult
					err    error
				}{result, err}
			}()
			t.Cleanup(func() {
				ctx.cancel()
				select {
				case <-completed:
				case <-time.After(2 * time.Second):
					t.Error("shell cleanup did not finish within cancellation bound")
				}
			})
			pid := awaitLocalPID(t, filepath.Join(directory, "leaf.pid"), completed)
			started := time.Now()
			ctx.cancel()
			select {
			case got := <-response:
				if mode == "cancel" {
					if !errors.Is(got.err, context.Canceled) || got.result.TimedOut || got.result.ErrorMessage != "command canceled" {
						t.Fatalf("cancellation did not preserve context semantics: %v, %#v", got.err, got.result)
					}
				} else if got.err != nil || !got.result.TimedOut || got.result.ExitCode != -1 {
					t.Fatalf("timeout not reported: %v, %#v", got.err, got.result)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("shell cancellation did not return within bound")
			}
			assertLocalStopped(t, pid)
			t.Logf("shell %s returned in %s after readiness and cancellation with descendant stopped", mode, time.Since(started))
		})
	}
}

func TestExecutorUTF8Output(t *testing.T) {
	executor, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	command := "printf '你好世界🌍'"
	if runtime.GOOS == "windows" {
		command = "Write-Output '你好世界🌍'"
	}
	result, err := executor.Run(context.Background(), domainexecution.RunRequest{Command: command})
	if err != nil || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "你好世界🌍" {
		t.Fatalf("UTF-8 shell output mismatch: %v, %#v", err, result)
	}
}

func TestLimitedBufferKeepsUTF8HeadAndTailWithinLimit(t *testing.T) {
	buffer := newLimitedBuffer(32)
	content := []byte("HEAD-" + strings.Repeat("你", 40) + "-TAIL")
	for offset := 0; offset < len(content); offset += 5 {
		if count, err := buffer.Write(content[offset:min(offset+5, len(content))]); err != nil || count != min(5, len(content)-offset) {
			t.Fatal("bounded writer did not accept full input")
		}
	}
	text := buffer.String()
	if !buffer.Truncated() || len(text) > 32 || !utf8.ValidString(text) || !strings.HasPrefix(text, "HEAD-") || !strings.HasSuffix(text, "-TAIL") {
		t.Fatalf("invalid bounded head/tail output: %q", text)
	}
	for _, content := range []string{"你好世界", "plain-ascii", ""} {
		buffer := newLimitedBuffer(64)
		for _, part := range []byte(content) {
			_, _ = buffer.Write([]byte{part})
		}
		if buffer.String() != content || buffer.Truncated() {
			t.Fatal("untruncated output changed across write boundaries")
		}
	}
}

func localHelperCommand(mode string, arguments ...string) string {
	parts := []string{os.Args[0], "-test.run=^TestLocalProcessHelper$", "--", "local-process-helper", mode}
	parts = append(parts, arguments...)
	for index, part := range parts {
		parts[index] = "'" + strings.ReplaceAll(part, "'", "'\\''") + "'"
		if runtime.GOOS == "windows" {
			parts[index] = "'" + strings.ReplaceAll(part, "'", "''") + "'"
		}
	}
	prefix := ""
	if runtime.GOOS == "windows" {
		prefix = "& "
	}
	return prefix + strings.Join(parts, " ")
}

func TestLocalProcessHelper(t *testing.T) {
	index := -1
	for i, value := range os.Args {
		if value == "local-process-helper" {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	args := os.Args[index+1:]
	switch args[0] {
	case "environment":
		if _, present := os.LookupEnv("ZZ_TEST_SECRET"); present {
			os.Exit(7)
		}
		os.Exit(0)
	case "leaf":
		_ = os.WriteFile(filepath.Join(args[1], "leaf.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
		time.Sleep(15 * time.Second)
		os.Exit(0)
	case "tree":
		cmd := exec.Command(os.Args[0], "-test.run=^TestLocalProcessHelper$", "--", "local-process-helper", "leaf", args[1])
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(8)
		}
		os.Exit(0)
	default:
		os.Exit(9)
	}
}

func awaitLocalPID(t *testing.T, path string, completed <-chan struct{}) int {
	t.Helper()
	deadline := time.Now().Add(50 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
				return pid
			}
		}
		select {
		case <-completed:
			t.Fatal("local helper exited before PID readiness")
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatal(fmt.Sprintf("local helper did not report PID at %s within startup budget", filepath.Base(path)))
	return 0
}

func TestExecutorDenylistIsOnlyReportedAsLocalPolicy(t *testing.T) {
	executor, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.Run(context.Background(), domainexecution.RunRequest{Command: "Remove-Item -Recurse C:\\"})
	if err == nil || !strings.Contains(err.Error(), "local executor policy") {
		t.Fatalf("expected explicit local policy error, got %v", err)
	}
}
