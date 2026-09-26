package lifecycle

import (
	"context"
	"testing"

	generationcommand "myai/core/application/chat/generation/command"
	"myai/core/hook"
)

func TestBridgeMapsStopPromptAndDeny(t *testing.T) {
	manager := &hook.Manager{}
	manager.Register(hookHandlerFunc(func(ctx context.Context, event hook.Event) (hook.Result, error) {
		if event.Type != hook.EventStop || event.Prompt != "final answer" {
			t.Fatalf("unexpected stop event: %+v", event)
		}
		return hook.Result{Decision: hook.DecisionDeny, Message: "continue with tests"}, nil
	}))

	outcome, err := (Bridge{Hooks: manager}).Handle(context.Background(), generationcommand.TurnHook{
		Kind:          generationcommand.TurnHookStop,
		SessionID:     "session-1",
		LastAssistant: "final answer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Denied || outcome.Continuation != "continue with tests" {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
}

func TestBridgeNilManagerIsNoop(t *testing.T) {
	outcome, err := (Bridge{}).Handle(context.Background(), generationcommand.TurnHook{Kind: generationcommand.TurnHookStop})
	if err != nil || outcome.Denied || outcome.Continuation != "" {
		t.Fatalf("expected noop outcome, got %+v err=%v", outcome, err)
	}
}

type hookHandlerFunc func(ctx context.Context, event hook.Event) (hook.Result, error)

func (f hookHandlerFunc) HandleHook(ctx context.Context, event hook.Event) (hook.Result, error) {
	return f(ctx, event)
}
