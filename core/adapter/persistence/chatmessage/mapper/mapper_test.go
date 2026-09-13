package mapper

import (
	"fmt"
	"testing"
	"time"

	generationcommand "myai/core/application/chat/generation/command"
	domainmessage "myai/core/domain/message"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func TestMapperConvertsMemoryMessagesAtPersistenceBoundary(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	mapper := Mapper{
		IDs: &sequentialIDs{},
		Now: func() time.Time {
			return now
		},
	}
	current := &session.Session{
		ID: "session-1",
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
			domainmessage.RuntimeInstruction("plan rules"),
			domainmessage.Text(domainmessage.RoleUser, "hello"),
			domainmessage.ToolCallMessage([]domainmessage.ToolCall{{ID: "call-1", Name: "read_file", Arguments: `{}`}}),
			domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "call-1", Name: "read_file", Content: "content"}),
		},
	}

	records := mapper.MemoryMessages(current)
	if len(records) != 4 {
		t.Fatalf("record count = %d, want 4", len(records))
	}
	if records[0].ID != "id-1" || records[0].Role != repository.RoleSystem || records[0].SyntheticReason != "runtime_instruction" {
		t.Fatalf("unexpected runtime record: %#v", records[0])
	}
	if records[1].ID != "id-2" || records[1].Role != repository.RoleUser || records[1].Content != "hello" {
		t.Fatalf("unexpected user record: %#v", records[1])
	}
	if records[2].Role != repository.RoleToolCall || records[2].ToolCallID != "call-1" || records[2].ToolName != "read_file" {
		t.Fatalf("unexpected tool call record: %#v", records[2])
	}
	if records[3].Role != repository.RoleTool || records[3].Content != "content" || records[3].ToolStatus != "success" {
		t.Fatalf("unexpected tool result record: %#v", records[3])
	}
	if !records[3].CreatedAt.Equal(now.Add(-time.Nanosecond)) {
		t.Fatalf("unexpected record time: %v", records[3].CreatedAt)
	}
	for index := 1; index < len(records); index++ {
		if records[index].Sequence <= records[index-1].Sequence {
			t.Fatalf("message sequence is not increasing: %#v", records)
		}
	}
}

func TestMapperPersistsEveryToolCallPart(t *testing.T) {
	mapper := Mapper{IDs: &sequentialIDs{}}
	current := &session.Session{
		ID: "session-1",
		Messages: []domainmessage.Message{
			domainmessage.ToolCallMessage([]domainmessage.ToolCall{
				{ID: "call-1", Name: "read_file", Arguments: `{"path":"a"}`},
				{ID: "call-2", Name: "read_file", Arguments: `{"path":"b"}`},
			}),
		},
	}

	records := mapper.MemoryMessages(current)
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}
	if records[0].ToolCallID != "call-1" || records[1].ToolCallID != "call-2" {
		t.Fatalf("unexpected tool call records: %#v", records)
	}
}

func TestMapperKeepsLiveMessageIdentityStableAcrossReadsAndPersistence(t *testing.T) {
	createdAt := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	message := domainmessage.Text(domainmessage.RoleUser, "hello")
	message.ID = "message-live-1"
	message.Sequence = 42
	message.CreatedAt = createdAt
	current := &session.Session{ID: "session-1", Messages: []domainmessage.Message{message}}
	mapper := Mapper{IDs: &sequentialIDs{}, Now: func() time.Time { return createdAt.Add(time.Hour) }}

	first := mapper.MemoryMessages(current)
	second := mapper.MemoryMessages(current)
	persisted := mapper.UserTurn(generationcommand.PersistUserMessage{
		SessionID: "session-1", Input: "hello", AppendedMessages: []domainmessage.Message{message},
	}, createdAt.Add(time.Minute))

	for name, records := range map[string][]repository.MessageRecord{"first": first, "second": second, "persisted": persisted} {
		if len(records) != 1 || records[0].ID != message.ID || records[0].Sequence != message.Sequence || !records[0].CreatedAt.Equal(createdAt) {
			t.Fatalf("%s records lost stable identity: %#v", name, records)
		}
	}
}

func TestMapperPersistsSyntheticUserReason(t *testing.T) {
	mapper := Mapper{IDs: &sequentialIDs{}}
	current := &session.Session{
		ID: "session-1",
		Messages: []domainmessage.Message{
			domainmessage.SyntheticUserText(domainmessage.SyntheticReasonSubagentResult, "subagent report"),
		},
	}

	records := mapper.MemoryMessages(current)
	if len(records) != 1 || records[0].Role != repository.RoleUser || records[0].SyntheticReason != "subagent_result" {
		t.Fatalf("unexpected synthetic user record: %#v", records)
	}
}

func TestMapperPersistsRAGContextAsEvidenceUserFragment(t *testing.T) {
	mapper := Mapper{IDs: &sequentialIDs{}}
	records := mapper.UserTurn(generationcommand.PersistUserMessage{
		SessionID: "session-1", RAGContext: "document evidence", Input: "question",
	}, time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC))
	if len(records) != 2 || records[0].Role != repository.RoleUser || records[0].SyntheticReason != string(domainmessage.SyntheticReasonRAGContext) {
		t.Fatalf("expected RAG context as synthetic user evidence, got %#v", records)
	}
}

func TestMapperSeparatesFullAndPromptLimitedToolResult(t *testing.T) {
	mapper := Mapper{IDs: &sequentialIDs{}}
	current := &session.Session{
		ID: "session-1",
		Messages: []domainmessage.Message{
			domainmessage.ToolResultMessage(domainmessage.ToolResult{
				ToolCallID: "call-1", Name: "read_file",
				Content: "bounded output", ErrorMessage: "bounded error",
				FullContent: "full audit output", FullErrorMessage: "full audit error",
				PromptTruncated: true,
			}),
		},
	}

	records := mapper.MemoryMessages(current)
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Content != "full audit output" || record.ToolError != "full audit error" {
		t.Fatalf("full audit output was not persisted: %#v", record)
	}
	if record.ToolPromptContent != "bounded output" || record.ToolPromptError != "bounded error" || !record.ToolPromptTruncated {
		t.Fatalf("bounded prompt output was not persisted separately: %#v", record)
	}
}

type sequentialIDs struct {
	next int
}

func (g *sequentialIDs) NewID() string {
	g.next++
	return fmt.Sprintf("id-%d", g.next)
}
