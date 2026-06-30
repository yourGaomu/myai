package hook

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
)

func TestAggregatePreToolUse(t *testing.T) {
	got := aggregatePreToolUse([]Result{
		{Decision: DecisionAllow},
		{Decision: DecisionAsk, Message: "confirm"},
	})
	if got.Decision != DecisionAllow || got.Message != "confirm" {
		t.Fatalf("unexpected aggregate result: %+v", got)
	}

	got = aggregatePreToolUse([]Result{
		{Decision: DecisionAsk},
		{Decision: DecisionDeny, Message: "blocked"},
	})
	if got.Decision != DecisionDeny || got.Message != "blocked" {
		t.Fatalf("unexpected deny result: %+v", got)
	}
}

func TestCommandHookHandlesJSONResult(t *testing.T) {
	dir := t.TempDir()
	command := `printf '{"decision":"deny","message":"blocked"}'`
	if runtime.GOOS == "windows" {
		command = `Write-Output '{"decision":"deny","message":"blocked"}'`
	}

	hook, err := NewCommandHook(CommandHookConfig{
		Event:   string(EventPreToolUse),
		Command: command,
	}, dir)
	if err != nil {
		t.Fatalf("new command hook: %v", err)
	}

	result, err := hook.HandleHook(context.Background(), Event{Type: EventPreToolUse})
	if err != nil {
		t.Fatalf("handle hook: %v", err)
	}
	if result.Decision != DecisionDeny || result.Message != "blocked" {
		t.Fatalf("unexpected hook result: %+v", result)
	}
}

func TestManagerEmitAndPreToolUse(t *testing.T) {
	var seen []Event
	manager := &Manager{}
	manager.Register(handlerFunc(func(ctx context.Context, event Event) (Result, error) {
		seen = append(seen, event)
		return Result{Decision: DecisionAllow}, nil
	}))

	result, err := manager.PreToolUse(context.Background(), Event{ToolName: "edit_file"})
	if err != nil {
		t.Fatalf("pre tool use: %v", err)
	}
	if result.Decision != DecisionAllow {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(seen) != 1 || seen[0].Type != EventPreToolUse || seen[0].ToolName != "edit_file" {
		t.Fatalf("unexpected seen events: %+v", seen)
	}

	if err := manager.Emit(context.Background(), Event{Type: EventSessionChanged, SessionID: "s1"}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if len(seen) != 2 || seen[1].Type != EventSessionChanged {
		t.Fatalf("unexpected emit events: %+v", seen)
	}
}

type handlerFunc func(ctx context.Context, event Event) (Result, error)

func (f handlerFunc) HandleHook(ctx context.Context, event Event) (Result, error) {
	return f(ctx, event)
}

func TestCommandHookIgnoresDisabled(t *testing.T) {
	disabled := false
	_, err := NewCommandHook(CommandHookConfig{
		Event:   string(EventPreToolUse),
		Command: "echo hi",
		Enabled: &disabled,
	}, t.TempDir())
	if err == nil {
		t.Fatalf("expected disabled hook error")
	}
}

func TestNormalizeDecision(t *testing.T) {
	if normalizeDecision(" allow ") != DecisionAllow {
		t.Fatalf("normalize allow failed")
	}
	if normalizeDecision("unknown") != DecisionContinue {
		t.Fatalf("normalize continue failed")
	}
}

func TestEventJSON(t *testing.T) {
	data, err := json.Marshal(normalizeEvent(Event{Type: EventSessionChanged}))
	if err != nil || len(data) == 0 {
		t.Fatalf("marshal event failed: %v", err)
	}
}
