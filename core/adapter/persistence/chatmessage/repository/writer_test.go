package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	generationcommand "myai/core/application/chat/generation/command"
	sessioncommand "myai/core/application/session/command"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func TestWriterSavesUserMessageAndSession(t *testing.T) {
	messages := &recordingMessageSaver{}
	sessions := &recordingSessionPersistence{}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	writer := Writer{
		Messages: messages,
		Sessions: sessions,
		IDs:      &sequentialIDs{},
		Now:      func() time.Time { return now },
	}

	err := writer.SaveUserMessage(context.Background(), generationcommand.PersistUserMessage{
		SessionID:          "session-1",
		Model:              "gpt-5",
		Title:              "title",
		Input:              "hello",
		RuntimeInstruction: "plan rules",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sessions.command.SessionID != "session-1" || sessions.command.Title != "title" {
		t.Fatalf("unexpected session command: %#v", sessions.command)
	}
	if len(messages.records) != 2 || messages.records[0].ID != "id-1" || messages.records[0].Role != repository.RoleSystem || messages.records[0].SyntheticReason != "runtime_instruction" {
		t.Fatalf("unexpected message records: %#v", messages.records)
	}
	if !messages.records[0].CreatedAt.Equal(now) {
		t.Fatalf("unexpected created time: %#v", messages.records[0].CreatedAt)
	}
	if messages.records[1].ID != "id-2" || messages.records[1].Role != repository.RoleUser || messages.records[1].Content != "hello" {
		t.Fatalf("unexpected user record: %#v", messages.records[1])
	}
	if !messages.records[1].CreatedAt.Equal(now.Add(time.Nanosecond)) {
		t.Fatalf("unexpected user created time: %#v", messages.records[1].CreatedAt)
	}
	if messages.records[0].Sequence != now.UnixNano() || messages.records[1].Sequence != now.Add(time.Nanosecond).UnixNano() {
		t.Fatalf("unexpected message sequences: %#v", messages.records)
	}
}

func TestWriterSavesAssistantMessageAndSessionSnapshot(t *testing.T) {
	messages := &recordingMessageSaver{}
	sessions := &recordingSessionPersistence{}
	writer := Writer{
		Messages: messages,
		Sessions: sessions,
		IDs:      &sequentialIDs{},
	}
	current := &session.Session{
		ID:             "session-1",
		Model:          "gpt-5",
		AgentMode:      session.AgentModePlan,
		PermissionMode: session.PermissionModeFull,
		ContextWindowK: 16,
		Summary:        "summary",
	}

	err := writer.SaveAssistantMessage(context.Background(), current, modelport.ChatResult{
		Content:   "answer",
		Reasoning: "reasoning",
		Usage:     modelport.TokenUsage{TotalTokens: 3, Available: true},
	}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages.records) != 1 || messages.records[0].ID != "id-1" || messages.records[0].Role != repository.RoleAssistant {
		t.Fatalf("unexpected assistant records: %#v", messages.records)
	}
	if sessions.record.ID != "session-1" || sessions.record.AgentMode != string(session.AgentModePlan) || sessions.record.Summary != "summary" {
		t.Fatalf("unexpected session snapshot: %#v", sessions.record)
	}
	if sessions.record.Usage != nil {
		t.Fatalf("session snapshot should use accumulated session usage, got %#v", sessions.record.Usage)
	}
}

func TestWriterReplacesRegeneratedTranscriptThroughAtomicRepository(t *testing.T) {
	transcripts := &recordingTranscriptRepository{}
	writer := Writer{
		Transcripts: transcripts,
		Sessions:    &recordingSessionPersistence{},
		IDs:         &sequentialIDs{},
	}
	current := &session.Session{
		ID:    "session-1",
		Model: "gpt-5",
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleUser, "hello"),
			domainmessage.ToolCallMessage([]domainmessage.ToolCall{
				{ID: "call-1", Name: "read_file", Arguments: `{"path":"a"}`},
				{ID: "call-2", Name: "read_file", Arguments: `{"path":"b"}`},
			}),
		},
	}

	if err := writer.ReplaceSessionMessages(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	if transcripts.snapshot.Session.ID != "session-1" || len(transcripts.snapshot.Messages) != 3 {
		t.Fatalf("unexpected transcript snapshot: %#v", transcripts.snapshot)
	}
	if transcripts.snapshot.Messages[1].ToolCallID != "call-1" || transcripts.snapshot.Messages[2].ToolCallID != "call-2" {
		t.Fatalf("batch tool calls were not preserved: %#v", transcripts.snapshot.Messages)
	}
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

type recordingTranscriptRepository struct {
	snapshot repository.TranscriptSnapshot
}

func (r *recordingTranscriptRepository) ReplaceSessionTranscript(_ context.Context, snapshot repository.TranscriptSnapshot) error {
	r.snapshot = snapshot
	return nil
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

type sequentialIDs struct {
	next int
}

func (g *sequentialIDs) NewID() string {
	g.next++
	return fmt.Sprintf("id-%d", g.next)
}
