package lifecycle

import (
	"context"
	"errors"
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

func TestStopDiagnosticsDoNotRequestContinuation(t *testing.T) {
	for _, decision := range []hook.Decision{hook.DecisionContinue, hook.DecisionAllow, hook.DecisionAsk, ""} {
		manager := &hook.Manager{}
		manager.Register(hookHandlerFunc(func(context.Context, hook.Event) (hook.Result, error) {
			return hook.Result{Decision: decision, Message: "ordinary diagnostic"}, nil
		}))
		outcome, err := (Bridge{Hooks: manager}).Handle(context.Background(), generationcommand.TurnHook{Kind: generationcommand.TurnHookStop, LastAssistant: "final answer"})
		if err != nil || outcome.Denied || outcome.Continuation != "" {
			t.Fatalf("diagnostic continued generation: %+v err=%v", outcome, err)
		}
	}
}

func TestStopFailurePreservesAnswerButPromptGateFailsClosed(t *testing.T) {
	manager := &hook.Manager{}
	manager.Register(hookHandlerFunc(func(context.Context, hook.Event) (hook.Result, error) {
		return hook.Result{}, errors.New("dummy hook failure")
	}))
	var reported error
	bridge := Bridge{Hooks: manager, OnError: func(err error) { reported = err }}
	outcome, err := bridge.Handle(context.Background(), generationcommand.TurnHook{Kind: generationcommand.TurnHookStop, LastAssistant: "final answer"})
	if err != nil || reported == nil || outcome.Denied || outcome.Continuation != "" {
		t.Fatalf("stop failure discarded answer: %+v %v reported=%v", outcome, err, reported)
	}
	if _, err := bridge.Handle(context.Background(), generationcommand.TurnHook{Kind: generationcommand.TurnHookUserPromptSubmit, Prompt: "hello"}); err == nil {
		t.Fatal("prompt gate failed open")
	}
}

func TestStopDenyWithoutMessageStillRequestsContinuation(t *testing.T) {
	manager := &hook.Manager{}
	manager.Register(hookHandlerFunc(func(context.Context, hook.Event) (hook.Result, error) {
		return hook.Result{Decision: hook.DecisionDeny}, nil
	}))
	outcome, err := (Bridge{Hooks: manager}).Handle(context.Background(), generationcommand.TurnHook{Kind: generationcommand.TurnHookStop})
	if err != nil || !outcome.Denied || outcome.Continuation == "" {
		t.Fatalf("explicit deny ignored: %+v %v", outcome, err)
	}
}

type hookHandlerFunc func(ctx context.Context, event hook.Event) (hook.Result, error)

func (f hookHandlerFunc) HandleHook(ctx context.Context, event hook.Event) (hook.Result, error) {
	return f(ctx, event)
}
