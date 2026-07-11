package generation

import (
	"context"
	"testing"
	"time"

	chatmessagerepository "myai/core/adapter/persistence/chatmessage/repository"
	generationcommand "myai/core/application/chat/generation/command"
	runtimeservice "myai/core/application/runtime/service"
	sessioncommand "myai/core/application/session/command"
	currentservice "myai/core/application/session/current/service"
	cacheport "myai/core/port/cache"
	modelport "myai/core/port/model"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func TestPersistenceDelegatesAssistantAndCurrentSession(t *testing.T) {
	messages := &recordingMessageSaver{}
	sessions := &recordingSessionPersistence{}
	cache := &recordingCurrentSessionCache{}
	persistence := Persistence{
		Messages: chatmessagerepository.Writer{
			Messages: messages,
			Sessions: sessions,
		},
		CurrentSession: currentservice.SessionService{
			Cache:  cache,
			UserID: "user-1",
			TTL:    time.Hour,
		},
		Async: runtimeservice.AsyncTaskService{Executor: inlineExecutor{}},
	}

	current := &session.Session{ID: "session-1", Model: "gpt-test"}
	persistence.PersistAssistant(current, modelport.ChatResult{Content: "answer"})
	persistence.PersistCurrentSession(current.ID)

	if len(messages.records) != 1 || messages.records[0].Content != "answer" {
		t.Fatalf("unexpected messages: %#v", messages.records)
	}
	if sessions.record.ID != current.ID {
		t.Fatalf("unexpected session record: %#v", sessions.record)
	}
	if cache.sessionID != current.ID || cache.userID != "user-1" {
		t.Fatalf("unexpected cache write: %#v", cache)
	}
}

func TestSummaryStoreUpdatesMemoryAndPersistence(t *testing.T) {
	memory := &recordingSummaryMemory{}
	sessions := &recordingSessionPersistence{}
	store := SummaryStore{Memory: memory, Sessions: sessions}
	current := &session.Session{ID: "session-1", Model: "gpt-test"}

	if err := store.SaveSummary(context.Background(), current, "summary", 4); err != nil {
		t.Fatalf("SaveSummary() error = %v", err)
	}
	if memory.sessionID != current.ID || memory.summary != "summary" || memory.compacted != 4 {
		t.Fatalf("unexpected memory update: %#v", memory)
	}
	if sessions.command.SessionID != current.ID || sessions.command.Model != current.Model {
		t.Fatalf("unexpected persistence command: %#v", sessions.command)
	}
}

func TestUserMessagePersistenceDelegatesAsyncSave(t *testing.T) {
	messages := &recordingMessageSaver{}
	sessions := &recordingSessionPersistence{}
	persistence := UserMessagePersistence{
		Messages: chatmessagerepository.Writer{
			Messages: messages,
			Sessions: sessions,
		},
		Async: runtimeservice.AsyncTaskService{Executor: inlineExecutor{}},
	}

	persistence.PersistUserMessage(generationcommand.PersistUserMessage{
		SessionID: "session-1",
		Model:     "model-1",
		Title:     "title",
		Input:     "hello",
	})

	if len(messages.records) != 1 || messages.records[0].Content != "hello" {
		t.Fatalf("unexpected messages: %#v", messages.records)
	}
	if sessions.command.SessionID != "session-1" || sessions.command.Title != "title" {
		t.Fatalf("unexpected session command: %#v", sessions.command)
	}
}

type inlineExecutor struct{}

func (inlineExecutor) Submit(task func()) error {
	task()
	return nil
}

type recordingMessageSaver struct {
	records []repository.MessageRecord
}

func (s *recordingMessageSaver) SaveMessage(_ context.Context, record repository.MessageRecord) error {
	s.records = append(s.records, record)
	return nil
}

type recordingSessionPersistence struct {
	command sessioncommand.SaveSession
	record  repository.SessionRecord
}

func (s *recordingSessionPersistence) Save(_ context.Context, command sessioncommand.SaveSession) error {
	s.command = command
	return nil
}

func (s *recordingSessionPersistence) SaveRecord(_ context.Context, record repository.SessionRecord) error {
	s.record = record
	return nil
}

type recordingCurrentSessionCache struct {
	userID    string
	sessionID string
	ttl       time.Duration
}

func (*recordingCurrentSessionCache) GetCurrentSession(context.Context, string) (string, error) {
	return "", nil
}

func (s *recordingCurrentSessionCache) SetCurrentSession(_ context.Context, userID string, sessionID string, ttl time.Duration) error {
	s.userID = userID
	s.sessionID = sessionID
	s.ttl = ttl
	return nil
}

func (*recordingCurrentSessionCache) DeleteCurrentSession(context.Context, string) error {
	return nil
}

var _ cacheport.CurrentSessionCache = (*recordingCurrentSessionCache)(nil)

type recordingSummaryMemory struct {
	sessionID string
	summary   string
	compacted int
}

func (m *recordingSummaryMemory) SetSummaryForSession(sessionID string, summary string, compactedMessages int) error {
	m.sessionID = sessionID
	m.summary = summary
	m.compacted = compactedMessages
	return nil
}
