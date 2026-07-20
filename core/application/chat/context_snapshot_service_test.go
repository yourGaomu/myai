package chat

import (
	"strings"
	"testing"

	domainmessage "myai/core/domain/message"
	"myai/core/session"
)

func TestContextSnapshotServiceUsesPersistedRuntimeInstruction(t *testing.T) {
	current := &session.Session{
		ID:             "session-1",
		AgentMode:      session.AgentModePlan,
		ContextWindowK: 16,
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, session.SystemPrompt()),
			domainmessage.RuntimeInstruction("plan prompt"),
			domainmessage.Text(domainmessage.RoleUser, "帮我写一个优美的古诗"),
		},
	}
	service := ContextSnapshotService{}

	snapshot := service.Snapshot(current)

	if snapshot.Info.PrefixHash == "" {
		t.Fatal("expected stable prefix hash")
	}
	if len(snapshot.Messages) != len(current.Messages) {
		t.Fatalf("snapshot added temporary messages: got %d want %d", len(snapshot.Messages), len(current.Messages))
	}

	runtimeIndex := -1
	for index, message := range snapshot.Messages {
		if message.IsSyntheticReason(domainmessage.SyntheticReasonRuntimeInstruction) && strings.Contains(message.Text(), domainmessage.RuntimeInstructionPrefix) {
			runtimeIndex = index
			break
		}
	}
	if runtimeIndex < 0 {
		t.Fatal("expected runtime instructions in selected messages")
	}
	if runtimeIndex+1 >= len(snapshot.Messages) || snapshot.Messages[runtimeIndex+1].Role != domainmessage.RoleUser {
		t.Fatal("expected runtime instructions immediately before the latest user message")
	}
}

func TestContextSnapshotServiceNilSessionReturnsEmptySnapshot(t *testing.T) {
	snapshot := ContextSnapshotService{}.Snapshot(nil)
	if len(snapshot.Messages) != 0 || snapshot.Info.WindowK != 0 {
		t.Fatalf("expected empty snapshot for nil session, got %#v", snapshot)
	}
}
