package events

import (
	"context"
	"errors"
	"strings"
	"testing"

	"myai/core/hook"
)

func TestPublisherEmitsSessionChanged(t *testing.T) {
	emitter := &fakeEmitter{}

	Publisher{Hooks: emitter}.SessionChanged(context.Background(), "session-1", "model")

	if len(emitter.events) != 1 {
		t.Fatalf("expected one event, got %#v", emitter.events)
	}
	event := emitter.events[0]
	if event.Type != hook.EventSessionChanged || event.SessionID != "session-1" || event.Reason != "model" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestPublisherEmitsSkillReloaded(t *testing.T) {
	emitter := &fakeEmitter{}

	Publisher{Hooks: emitter}.SkillReloaded(context.Background(), 3, "manual")

	if len(emitter.events) != 1 {
		t.Fatalf("expected one event, got %#v", emitter.events)
	}
	event := emitter.events[0]
	if event.Type != hook.EventSkillReloaded || event.SkillCount != 3 || event.Reason != "manual" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestPublisherReportsErrors(t *testing.T) {
	var reported error
	Publisher{
		Hooks: &fakeEmitter{err: errors.New("emit failed")},
		OnError: func(err error) {
			reported = err
		},
	}.SessionChanged(context.Background(), "session-1", "model")

	if reported == nil || !strings.Contains(reported.Error(), "session changed hook failed") {
		t.Fatalf("expected wrapped hook error, got %v", reported)
	}
}

type fakeEmitter struct {
	events []hook.Event
	err    error
}

func (e *fakeEmitter) Emit(ctx context.Context, event hook.Event) error {
	e.events = append(e.events, event)
	return e.err
}
