package hook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"myai/core/trust"
)

func TestCommandHookProtocolSeparatesDiagnosticsAndFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name, stdout, stderr string
		want                 Decision
		wantError            bool
	}{
		{"deny with stderr", `{"decision":"deny","message":"blocked"}`, "ordinary diagnostic", DecisionDeny, false},
		{"stderr alone", "", `{"decision":"deny"}`, DecisionContinue, false},
		{"plain stdout", "ordinary log", "", "", true},
		{"stdout prefix", "log\n" + `{"decision":"deny"}`, "", "", true},
		{"stdout suffix", `{"decision":"deny"}` + "\nlog", "", "", true},
		{"unknown decision", `{"decision":"denny"}`, "", "", true},
		{"null result", "null", "", "", true},
		{"unknown field", `{"decison":"deny"}`, "", "", true},
		{"null decision", `{"decision":null}`, "", "", true},
		{"empty decision", `{"decision":""}`, "", "", true},
		{"duplicate field", `{"decision":"deny","decision":"continue"}`, "", "", true},
		{"case field", `{"Decision":"deny"}`, "", "", true},
		{"rewrite scalar", `{"arguments":"null"}`, "", "", true},
		{"continue", `{"decision":"continue","message":"diagnostic"}`, "", DecisionContinue, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := outputCommand(test.stdout, test.stderr)
			handler, err := NewCommandHook(CommandHookConfig{Event: string(EventPreToolUse), Command: command, Timeout: "30s"}, t.TempDir(), WithCommandTrustPolicy(&trust.Policy{Allow: true}))
			if err != nil {
				t.Fatal(err)
			}
			result, err := handler.HandleHook(context.Background(), Event{Type: EventPreToolUse})
			if (err != nil) != test.wantError {
				t.Fatalf("expected error=%v, got %v", test.wantError, err)
			}
			if !test.wantError && result.Decision != test.want {
				t.Fatalf("unexpected decision %q", result.Decision)
			}
		})
	}
}

func outputCommand(stdout, stderr string) string {
	// Test values are fixed protocol fixtures, never attacker-controlled scripts.
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("[Console]::Out.Write('%s'); [Console]::Error.Write('%s')", strings.ReplaceAll(stdout, "'", "''"), strings.ReplaceAll(stderr, "'", "''"))
	}
	return fmt.Sprintf("printf '%%s' '%s'; printf '%%s' '%s' >&2", strings.ReplaceAll(stdout, "'", "'\"'\"'"), strings.ReplaceAll(stderr, "'", "'\"'\"'"))
}

func TestCommandHookExitTwoRemainsDeny(t *testing.T) {
	handler, err := NewCommandHook(CommandHookConfig{Event: string(EventPreToolUse), Command: outputCommand("", "blocked") + "; exit 2", Timeout: "30s"}, t.TempDir(), WithCommandTrustPolicy(&trust.Policy{Allow: true}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.HandleHook(context.Background(), Event{Type: EventPreToolUse})
	if err != nil || result.Decision != DecisionDeny || result.Message != "blocked" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestManagerChainsRewritesAndStopsAtDeny(t *testing.T) {
	manager := &Manager{}
	manager.Register(handlerFunc(func(_ context.Context, event Event) (Result, error) {
		if event.ToolArguments != `{"count":1}` {
			t.Fatal("first hook lost original arguments")
		}
		return Result{Arguments: `{"count":2}`}, nil
	}))
	manager.Register(handlerFunc(func(_ context.Context, event Event) (Result, error) {
		if event.ToolArguments != `{"count":2}` {
			t.Fatal("second hook did not see rewrite")
		}
		return Result{Decision: DecisionDeny, Message: "blocked rewrite"}, nil
	}))
	manager.Register(handlerFunc(func(_ context.Context, event Event) (Result, error) {
		t.Fatal("hook ran after deny")
		return Result{}, nil
	}))
	result, err := manager.PreToolUse(context.Background(), Event{ToolArguments: `{"count":1}`})
	if err != nil || result.Decision != DecisionDeny {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestManagerRejectsInvalidRewriteAndUnknownDecision(t *testing.T) {
	for _, result := range []Result{{Decision: "unknown"}, {Arguments: "[]"}, {Arguments: "null"}, {Arguments: "{"}} {
		manager := &Manager{}
		manager.Register(handlerFunc(func(context.Context, Event) (Result, error) { return result, nil }))
		manager.Register(handlerFunc(func(context.Context, Event) (Result, error) {
			t.Fatal("invalid result reached downstream hook")
			return Result{}, nil
		}))
		if _, err := manager.PreToolUse(context.Background(), Event{}); err == nil {
			t.Fatalf("invalid result accepted: %+v", result)
		}
	}
}

func TestManagerValidatesEachRewriteWhenSchemaBoundaryProvided(t *testing.T) {
	called := false
	manager, err := NewManager(Config{ValidateArguments: func(name, args string) error {
		called = name == "dummy" && args == `{"count":"invalid"}`
		return errors.New("count must be an integer")
	}})
	if err != nil {
		t.Fatal(err)
	}
	manager.Register(handlerFunc(func(context.Context, Event) (Result, error) { return Result{Arguments: `{"count":"invalid"}`}, nil }))
	manager.Register(handlerFunc(func(context.Context, Event) (Result, error) {
		t.Fatal("schema rejected rewrite reached downstream hook")
		return Result{}, nil
	}))
	if _, err := manager.PreToolUse(context.Background(), Event{ToolName: "dummy"}); err == nil || !called {
		t.Fatalf("schema boundary not enforced: %v", err)
	}
}

func TestCommandHookRequiresApprovalAndRechecksRevocation(t *testing.T) {
	workspace, userDir := t.TempDir(), t.TempDir()
	marker := filepath.Join(workspace, "ran.txt")
	command := "printf safe > ran.txt"
	if runtime.GOOS == "windows" {
		command = "[System.IO.File]::WriteAllText('ran.txt','safe')"
	}
	store := trust.NewStore(filepath.Join(userDir, "trust.json"))
	handler, err := NewCommandHook(CommandHookConfig{Event: string(EventPreToolUse), Command: command, Timeout: "30s"}, workspace, WithCommandTrustPolicy(&trust.Policy{Store: store}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = handler.HandleHook(context.Background(), Event{Type: EventPreToolUse})
	if !errors.Is(err, trust.ErrUntrusted) {
		t.Fatalf("expected untrusted, got %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unapproved hook executed")
	}
	if err := store.Approve(handler.request); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.HandleHook(context.Background(), Event{Type: EventPreToolUse}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("approved hook did not execute")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(handler.request); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.HandleHook(context.Background(), Event{Type: EventPreToolUse}); !errors.Is(err, trust.ErrUntrusted) {
		t.Fatalf("revoked hook ran: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("revoked hook executed")
	}
}

func TestCommandHookTimeoutAndUTF8(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell-specific encoding and timeout regression")
	}
	handler, err := NewCommandHook(CommandHookConfig{Event: string(EventPreToolUse), Command: `$event = [Console]::In.ReadToEnd() | ConvertFrom-Json; @{decision='continue';message=$event.prompt} | ConvertTo-Json -Compress`, Timeout: "30s"}, t.TempDir(), WithCommandTrustPolicy(&trust.Policy{Allow: true}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.HandleHook(context.Background(), Event{Type: EventPreToolUse, Prompt: "中文内容"})
	if err != nil || result.Message != "中文内容" {
		t.Fatalf("UTF-8 round trip failed: %+v %v", result, err)
	}
	ready := filepath.Join(handler.workDir, "ready.txt")
	handler.command = "[System.IO.File]::WriteAllText('ready.txt','ready'); Start-Sleep -Seconds 60"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := handler.HandleHook(ctx, Event{Type: EventPreToolUse}); finished <- err }()
	startup := time.NewTimer(30 * time.Second)
	defer startup.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
readyLoop:
	for {
		if _, err := os.Stat(ready); err == nil {
			break readyLoop
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case err := <-finished:
			t.Fatalf("hook exited before ready handshake: %v", err)
		case <-startup.C:
			t.Fatal("hook did not reach ready handshake")
		case <-tick.C:
		}
	}
	start := time.Now()
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation not reported: %v", err)
		}
		if time.Since(start) > 3*time.Second {
			t.Fatal("hook cancellation was not bounded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hook cancellation was not bounded")
	}
	// Separately retain coverage for the configured timeout/error semantics.
	handler.command = "Start-Sleep -Seconds 60"
	handler.timeout = 100 * time.Millisecond
	start = time.Now()
	if _, err := handler.HandleHook(context.Background(), Event{Type: EventPreToolUse}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout not reported: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("hook timeout was not bounded")
	}
}

func TestHookOutputLimitAppliesDuringProcessPipeCopy(t *testing.T) {
	output := &limitedOutput{}
	count, err := io.Copy(output, strings.NewReader(strings.Repeat("a", maxHookOutputBytes+64)))
	if err != nil || count != maxHookOutputBytes+64 || !output.overflow || len(output.String()) != maxHookOutputBytes {
		t.Fatalf("pipe output not bounded: count=%d size=%d overflow=%v err=%v", count, len(output.String()), output.overflow, err)
	}
}

func TestCommandHookRejectsNonpositiveTimeout(t *testing.T) {
	for _, timeout := range []string{"0s", "-1s"} {
		if _, err := NewCommandHook(CommandHookConfig{Event: string(EventPreToolUse), Command: "unused", Timeout: timeout}, t.TempDir()); err == nil {
			t.Fatalf("timeout %q accepted", timeout)
		}
	}
}
