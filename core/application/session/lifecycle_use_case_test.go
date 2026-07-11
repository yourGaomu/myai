package sessionapp

import (
	"context"
	"testing"

	"myai/core/session"
)

func TestLifecycleUseCaseCreatePersistsCurrentSessionAndPublishesEvent(t *testing.T) {
	memory := lifecycleMemory("")
	persistence := &recordingSessionPersistence{}
	current := &recordingCurrentSession{}
	events := &recordingSessionEvents{}
	useCase := LifecycleUseCase{
		Lifecycle:   LifecycleService{Memory: memory},
		Persistence: persistence,
		Current:     current,
		Events:      events,
	}

	result, err := useCase.Create(context.Background(), CreateSessionCommand{})
	if err != nil {
		t.Fatal(err)
	}

	if result.SessionID != "new-session" || result.Current == nil || result.Action != SessionActionNew {
		t.Fatalf("unexpected create result: %#v", result)
	}
	if len(persistence.commands) != 1 || persistence.commands[0].Title != "New chat" {
		t.Fatalf("unexpected persistence commands: %#v", persistence.commands)
	}
	if current.sessionID != result.SessionID {
		t.Fatalf("current session = %q, want %q", current.sessionID, result.SessionID)
	}
	assertSessionEvents(t, events.events, sessionEvent{sessionID: result.SessionID, reason: SessionActionNew})
}

func TestLifecycleUseCaseDeleteCurrentLoadsAnotherSession(t *testing.T) {
	memory := lifecycleMemory("session-1")
	memory.sessions["session-2"] = &session.Session{ID: "session-2", Model: "gpt-5"}
	current := &recordingCurrentSession{}
	events := &recordingSessionEvents{}
	useCase := LifecycleUseCase{
		Lifecycle: LifecycleService{
			Memory:   memory,
			Sessions: &fakeLifecycleSessionRepository{},
		},
		Current: current,
		SessionQuery: fakeLifecycleSessionQuery{records: []SessionListItem{
			{ID: "session-2", Model: "gpt-5"},
		}},
		Events: events,
	}

	result, err := useCase.Delete(context.Background(), DeleteSessionCommand{})
	if err != nil {
		t.Fatal(err)
	}

	if result.SessionID != "session-1" || result.Current == nil || result.Current.ID != "session-2" {
		t.Fatalf("unexpected delete result: %#v", result)
	}
	if current.sessionID != "session-2" || memory.currentID != "session-2" {
		t.Fatalf("expected fallback session to become current, cache=%q memory=%q", current.sessionID, memory.currentID)
	}
	assertSessionEvents(t, events.events,
		sessionEvent{sessionID: "session-1", reason: SessionActionDelete},
		sessionEvent{sessionID: "session-2", reason: SessionActionLoad},
	)
}

func TestLifecycleUseCaseDeleteLastSessionCreatesReplacement(t *testing.T) {
	memory := lifecycleMemory("session-1")
	persistence := &recordingSessionPersistence{}
	current := &recordingCurrentSession{}
	events := &recordingSessionEvents{}
	useCase := LifecycleUseCase{
		Lifecycle: LifecycleService{
			Memory:   memory,
			Sessions: &fakeLifecycleSessionRepository{},
		},
		Persistence:  persistence,
		Current:      current,
		SessionQuery: fakeLifecycleSessionQuery{},
		Events:       events,
	}

	result, err := useCase.Delete(context.Background(), DeleteSessionCommand{})
	if err != nil {
		t.Fatal(err)
	}

	if result.Current == nil || result.Current.ID != "new-session" {
		t.Fatalf("expected replacement session, got %#v", result)
	}
	if len(persistence.commands) != 1 || persistence.commands[0].SessionID != "new-session" {
		t.Fatalf("unexpected persistence commands: %#v", persistence.commands)
	}
	if current.sessionID != "new-session" {
		t.Fatalf("current session = %q, want new-session", current.sessionID)
	}
	assertSessionEvents(t, events.events,
		sessionEvent{sessionID: "session-1", reason: SessionActionDelete},
		sessionEvent{sessionID: "new-session", reason: SessionActionNew},
	)
}

type recordingSessionPersistence struct {
	commands []SaveSessionCommand
	err      error
}

func (p *recordingSessionPersistence) Save(_ context.Context, command SaveSessionCommand) error {
	p.commands = append(p.commands, command)
	return p.err
}

type recordingCurrentSession struct {
	sessionID string
	err       error
}

func (s *recordingCurrentSession) Save(_ context.Context, sessionID string) error {
	s.sessionID = sessionID
	return s.err
}

type fakeLifecycleSessionQuery struct {
	records []SessionListItem
	err     error
}

func (q fakeLifecycleSessionQuery) ListSessions(context.Context, bool) ([]SessionListItem, error) {
	return q.records, q.err
}

type sessionEvent struct {
	sessionID string
	reason    string
}

type recordingSessionEvents struct {
	events []sessionEvent
}

func (p *recordingSessionEvents) SessionChanged(_ context.Context, sessionID string, reason string) {
	p.events = append(p.events, sessionEvent{sessionID: sessionID, reason: reason})
}

func assertSessionEvents(t *testing.T, actual []sessionEvent, expected ...sessionEvent) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("event count = %d, want %d: %#v", len(actual), len(expected), actual)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("event[%d] = %#v, want %#v", index, actual[index], expected[index])
		}
	}
}
