package session

import (
	"testing"

	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	domainmessage "myai/core/domain/message"
)

func TestNewFromStateRebuildsLegacySummarySourceHash(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"),
		domainmessage.Text(domainmessage.RoleUser, "request"),
		domainmessage.Text(domainmessage.RoleAssistant, "answer"),
	}
	current := NewFromState(InitialState{
		ID: "session-1", Summary: "unverified summary", CompactedMessages: 2,
		Messages: messages,
	})
	if current.Summary != "unverified summary" || current.CompactedMessages != 2 || current.CompactionSourceHash == "" {
		t.Fatalf("expected legacy summary hash to be rebuilt: %#v", current)
	}
}

func TestNewFromStateKeepsHashVerifiedSummary(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"),
		domainmessage.Text(domainmessage.RoleUser, "request"),
		domainmessage.Text(domainmessage.RoleAssistant, "answer"),
	}
	hash := contextmgr.CompactionSourceHash(messages, 3)
	current := NewFromState(InitialState{
		ID: "session-1", Summary: "verified summary", CompactedMessages: 3,
		CompactionSourceHash: hash, Messages: messages,
	})
	if current.Summary != "verified summary" || current.CompactedMessages != 3 || current.CompactionSourceHash != hash {
		t.Fatalf("expected verified summary to survive: %#v", current)
	}
}

func TestNewFromStateRebuildsLegacyCheckpointSourceHash(t *testing.T) {
	checkpoint := compaction.LegacyCheckpoint("legacy checkpoint", 2, "")
	current := NewFromState(InitialState{
		ID: "session-1", CompactionCheckpoint: &checkpoint,
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
			domainmessage.Text(domainmessage.RoleUser, "request"),
			domainmessage.Text(domainmessage.RoleAssistant, "answer"),
		},
	})
	if current.CompactionCheckpoint == nil || current.CompactionCheckpoint.SourceHistoryHash == "" || current.Summary != "legacy checkpoint" {
		t.Fatalf("expected legacy checkpoint hash to be rebuilt: session=%#v checkpoint=%+v", current, current.CompactionCheckpoint)
	}
}

func TestSessionAssignsStableIdentityWhenAppendingMessages(t *testing.T) {
	current := NewFromState(InitialState{ID: "session-1"})
	current.AddUserMessage("hello")
	first := current.Messages[len(current.Messages)-1]
	if first.ID == "" || first.Sequence <= 0 || first.CreatedAt.IsZero() {
		t.Fatalf("message identity was not assigned: %#v", first)
	}
	current.AddAssistantMessage("answer")
	second := current.Messages[len(current.Messages)-1]
	if second.ID == "" || second.ID == first.ID || second.Sequence <= first.Sequence || second.CreatedAt.Before(first.CreatedAt) {
		t.Fatalf("message identity is not monotonic: first=%#v second=%#v", first, second)
	}
}
