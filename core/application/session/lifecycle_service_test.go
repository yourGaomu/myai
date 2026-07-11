package sessionapp

import (
	"context"
	"errors"
	"testing"
	"time"

	domainmessage "myai/core/domain/message"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func TestLifecycleServiceNewSession(t *testing.T) {
	memory := lifecycleMemory("")

	current, err := (LifecycleService{Memory: memory}).NewSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if current.ID != "new-session" || memory.currentID != "new-session" {
		t.Fatalf("expected new session to become current, current=%#v memory=%#v", current, memory)
	}
}

func TestLifecycleServiceLoadSessionSetsCurrent(t *testing.T) {
	memory := lifecycleMemory("session-1")

	current, err := (LifecycleService{Memory: memory}).LoadSession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}

	if current.ID != "session-1" || memory.currentID != "session-1" {
		t.Fatalf("expected loaded session to become current, current=%#v memory=%#v", current, memory)
	}
}

func TestLifecycleServiceDeleteSessionReportsDeletedCurrent(t *testing.T) {
	memory := lifecycleMemory("session-1")
	store := &fakeLifecycleSessionRepository{}

	result, err := (LifecycleService{
		Memory:   memory,
		Sessions: store,
		Now:      fixedLifecycleTime,
	}).DeleteSession(context.Background(), DeleteSessionCommand{})
	if err != nil {
		t.Fatal(err)
	}

	if result.SessionID != "session-1" || !result.DeletedCurrent {
		t.Fatalf("expected current session deletion result, got %#v", result)
	}
	if _, err := memory.GetSession("session-1"); err == nil {
		t.Fatal("expected deleted session to be removed from memory")
	}
	if store.deletedID != "session-1" || !store.deletedAt.Equal(fixedLifecycleTime()) {
		t.Fatalf("expected store session to be marked deleted, got id=%q at=%v", store.deletedID, store.deletedAt)
	}
}

func TestLifecycleServiceRestoreSession(t *testing.T) {
	store := &fakeLifecycleSessionRepository{}

	restoredID, err := (LifecycleService{
		Sessions: store,
		Now:      fixedLifecycleTime,
	}).RestoreSession(context.Background(), RestoreSessionCommand{SessionID: " session-1 "})
	if err != nil {
		t.Fatal(err)
	}

	if restoredID != "session-1" {
		t.Fatalf("expected normalized restored id, got %q", restoredID)
	}
	if store.restoredID != "session-1" || !store.restoredAt.Equal(fixedLifecycleTime()) {
		t.Fatalf("expected store session to be marked restored, got id=%q at=%v", store.restoredID, store.restoredAt)
	}
}

func TestLifecycleServiceClearCurrent(t *testing.T) {
	memory := lifecycleMemory("session-1")
	messages := &fakeLifecycleMessageRepository{}
	memory.sessions["session-1"].Summary = "summary"
	memory.sessions["session-1"].Messages = []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleUser, "hello"),
	}

	current, err := (LifecycleService{Memory: memory, Messages: messages}).ClearCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if messages.clearedID != "session-1" {
		t.Fatalf("expected persistent messages to be cleared, got %q", messages.clearedID)
	}
	if current.ID != "session-1" || current.Summary != "" {
		t.Fatalf("expected current session to be cleared, got %#v", current)
	}
	if len(current.Messages) != 1 || current.Messages[0].Role != domainmessage.RoleSystem {
		t.Fatalf("expected messages to be reset to system prompt, got %#v", current.Messages)
	}
}

func (s *fakeMemoryStore) NewSession() error {
	sessionID := "new-session"
	modelID := s.currentModelID
	if modelID == "" {
		modelID = "default"
	}
	s.currentID = sessionID
	s.currentModelID = modelID
	s.sessions[sessionID] = &session.Session{
		ID:             sessionID,
		Model:          modelID,
		AgentMode:      session.AgentModeChat,
		PermissionMode: session.PermissionModeAsk,
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, session.SystemPrompt()),
		},
	}
	return nil
}

func (s *fakeMemoryStore) Current() (*session.Session, error) {
	return s.GetSession(s.currentID)
}

func (s *fakeMemoryStore) CurrentSessionId() string {
	return s.currentID
}

func (s *fakeMemoryStore) ClearCurrent() error {
	current, err := s.Current()
	if err != nil {
		return err
	}
	current.Clear()
	return nil
}

func (s *fakeMemoryStore) RemoveSession(sessionID string) error {
	if s.sessions[sessionID] == nil {
		return errors.New("session not found")
	}
	delete(s.sessions, sessionID)
	if s.currentID == sessionID {
		s.currentID = ""
	}
	return nil
}

func lifecycleMemory(sessionID string) *fakeMemoryStore {
	memory := settingsMemory(sessionID)
	if memory.currentModelID == "" {
		memory.currentModelID = "default"
	}
	return memory
}

type fakeLifecycleSessionRepository struct {
	sessionErr error
	deletedID  string
	deletedAt  time.Time
	restoredID string
	restoredAt time.Time
}

func (r *fakeLifecycleSessionRepository) GetSession(ctx context.Context, sessionID string) (repository.SessionRecord, error) {
	if r.sessionErr != nil {
		return repository.SessionRecord{}, r.sessionErr
	}
	return repository.SessionRecord{ID: sessionID}, nil
}

func (r *fakeLifecycleSessionRepository) MarkSessionDeleted(ctx context.Context, sessionID string, deletedAt time.Time) error {
	r.deletedID = sessionID
	r.deletedAt = deletedAt
	return nil
}

func (r *fakeLifecycleSessionRepository) MarkSessionRestored(ctx context.Context, sessionID string, restoredAt time.Time) error {
	r.restoredID = sessionID
	r.restoredAt = restoredAt
	return nil
}

type fakeLifecycleMessageRepository struct {
	clearedID string
}

func (r *fakeLifecycleMessageRepository) ListMessages(ctx context.Context, sessionID string) ([]repository.MessageRecord, error) {
	return nil, nil
}

func (r *fakeLifecycleMessageRepository) ClearMessages(ctx context.Context, sessionID string) error {
	r.clearedID = sessionID
	return nil
}

func fixedLifecycleTime() time.Time {
	return time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
}
