package generation

import (
	"context"
	"errors"
	"strings"
	"testing"

	chatmessagerepository "myai/core/adapter/persistence/chatmessage/repository"
	generationcommand "myai/core/application/chat/generation/command"
	runtimeservice "myai/core/application/runtime/service"
	sessioncommand "myai/core/application/session/command"
	modelport "myai/core/port/model"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func TestPersistenceDelegatesAssistantSnapshot(t *testing.T) {
	messages := &recordingMessageSaver{}
	sessions := &recordingSessionPersistence{}
	persistence := Persistence{
		Messages: chatmessagerepository.Writer{
			Messages: messages,
			Sessions: sessions,
		},
		Async: runtimeservice.AsyncTaskService{Executor: inlineExecutor{}},
	}

	current := &session.Session{ID: "session-1", Model: "gpt-test"}
	persistence.PersistAssistant(current, modelport.ChatResult{Content: "answer"})

	if len(messages.records) != 1 || messages.records[0].Content != "answer" {
		t.Fatalf("unexpected messages: %#v", messages.records)
	}
	if sessions.record.ID != current.ID {
		t.Fatalf("unexpected session record: %#v", sessions.record)
	}
}

func TestPersistenceCapturesAssistantSessionBeforeQueueing(t *testing.T) {
	messages := &recordingMessageSaver{}
	sessions := &recordingSessionPersistence{}
	executor := &queuedExecutor{}
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{Executor: executor})
	persistence := Persistence{
		Messages: chatmessagerepository.Writer{Messages: messages, Sessions: sessions},
		Queue:    queue,
	}
	current := &session.Session{ID: "session-1", Model: "gpt-test", Summary: "submitted"}

	persistence.PersistAssistant(current, modelport.ChatResult{Content: "answer"})
	current.Summary = "mutated after submission"

	if len(executor.tasks) != 1 {
		t.Fatalf("expected one queued persistence task, got %d", len(executor.tasks))
	}
	executor.tasks[0]()
	if sessions.record.Summary != "submitted" {
		t.Fatalf("expected submitted snapshot, got %#v", sessions.record)
	}
}

func TestRegenerationPersistenceReturnsTranscriptFailure(t *testing.T) {
	expected := errors.New("transaction failed")
	reported := error(nil)
	persistence := RegenerationPersistence{
		Messages: failingTranscriptWriter{err: expected},
		Queue: NewSessionQueue(runtimeservice.AsyncTaskService{
			Executor: inlineExecutor{},
		}),
		OnError: func(err error) { reported = err },
	}

	err := persistence.PersistRegeneratedSession(context.Background(), &session.Session{ID: "session-1"})
	if !errors.Is(err, expected) || !errors.Is(reported, expected) {
		t.Fatalf("expected transaction error to be returned and reported: err=%v reported=%v", err, reported)
	}
	if !strings.Contains(err.Error(), "replace regenerated session transcript failed") {
		t.Fatalf("expected operation context, got %v", err)
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

func TestSummaryStoreRollsBackMemoryWhenPersistenceFails(t *testing.T) {
	memory := &recordingSummaryMemory{summary: "old", compacted: 2}
	expected := errors.New("durable session write failed")
	current := &session.Session{ID: "session-1", Model: "gpt-test", Summary: "old", CompactedMessages: 2}
	store := SummaryStore{Memory: memory, Sessions: failingSummaryPersistence{err: expected}}

	err := store.SaveSummary(context.Background(), current, "new", 5)
	if !errors.Is(err, expected) {
		t.Fatalf("expected persistence error, got %v", err)
	}
	if current.Summary != "old" || current.CompactedMessages != 2 {
		t.Fatalf("session state was not rolled back: %#v", current)
	}
	if memory.summary != "old" || memory.compacted != 2 {
		t.Fatalf("memory state was not rolled back: %#v", memory)
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

type failingTranscriptWriter struct {
	err error
}

func (w failingTranscriptWriter) ReplaceSessionMessages(context.Context, *session.Session) error {
	return w.err
}

func (s *recordingMessageSaver) SaveMessage(_ context.Context, record repository.MessageRecord) error {
	s.records = append(s.records, record)
	return nil
}

type recordingSessionPersistence struct {
	command sessioncommand.SaveSession
	record  repository.SessionRecord
}

type failingSummaryPersistence struct {
	err error
}

func (p failingSummaryPersistence) Save(context.Context, sessioncommand.SaveSession) error {
	return p.err
}

func (s *recordingSessionPersistence) Save(_ context.Context, command sessioncommand.SaveSession) error {
	s.command = command
	return nil
}

func (s *recordingSessionPersistence) SaveRecord(_ context.Context, record repository.SessionRecord) error {
	s.record = record
	return nil
}

func (s *recordingSessionPersistence) PrepareRecord(_ context.Context, record repository.SessionRecord) (repository.SessionRecord, error) {
	s.record = record
	return record, nil
}

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
