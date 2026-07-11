package chat

import (
	"strings"
	"testing"

	runtimeservice "myai/core/application/runtime/service"
	domainmessage "myai/core/domain/message"
	"myai/core/session"
)

func TestContextSnapshotServiceRuntimePromptDoesNotChangeCacheablePrefix(t *testing.T) {
	current := &session.Session{
		ID:             "session-1",
		AgentMode:      session.AgentModePlan,
		ContextWindowK: 16,
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, session.SystemPrompt()),
			domainmessage.Text(domainmessage.RoleUser, "帮我写一个优美的古诗"),
		},
	}
	service := ContextSnapshotService{}

	chatSnapshot := service.Snapshot(current, "")
	planSnapshot := service.Snapshot(current, runtimeservice.PlanModePrompt)

	if chatSnapshot.Info.PrefixHash == "" {
		t.Fatal("expected stable prefix hash")
	}
	if chatSnapshot.Info.PrefixHash != planSnapshot.Info.PrefixHash {
		t.Fatalf("expected runtime prompt to keep prefix hash stable, chat=%s plan=%s", chatSnapshot.Info.PrefixHash, planSnapshot.Info.PrefixHash)
	}
	if len(planSnapshot.Messages) != len(chatSnapshot.Messages)+1 {
		t.Fatalf("expected runtime message to be added for this turn")
	}

	runtimeIndex := -1
	for index, message := range planSnapshot.Messages {
		if message.Role == domainmessage.RoleSystem && strings.Contains(message.Text(), runtimeservice.RuntimeInstructionPrefix) {
			runtimeIndex = index
			break
		}
	}
	if runtimeIndex < 0 {
		t.Fatal("expected runtime instructions in selected messages")
	}
	if runtimeIndex+1 >= len(planSnapshot.Messages) || planSnapshot.Messages[runtimeIndex+1].Role != domainmessage.RoleUser {
		t.Fatal("expected runtime instructions immediately before the latest user message")
	}
}

func TestContextSnapshotServiceNilSessionReturnsEmptySnapshot(t *testing.T) {
	snapshot := ContextSnapshotService{}.Snapshot(nil, "runtime")
	if len(snapshot.Messages) != 0 || snapshot.Info.WindowK != 0 {
		t.Fatalf("expected empty snapshot for nil session, got %#v", snapshot)
	}
}
