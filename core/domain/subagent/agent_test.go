package subagent

import (
	"testing"
	"time"
)

func TestAgentMessageValidationRequiresRoutingAndDeliveryMetadata(t *testing.T) {
	message := AgentMessage{
		ID: "message-1", AuthorAgentID: "agent-parent", RecipientAgentID: "agent-child",
		Kind: AgentMessageKindInterAgent, Content: "inspect the implementation",
		Trigger: AgentMessageTriggerQueue, Status: AgentMessagePending,
		CreatedAt: time.Now().UTC(),
	}
	if err := message.Validate(); err != nil {
		t.Fatalf("valid agent message rejected: %v", err)
	}

	message.Trigger = "invalid"
	if err := message.Validate(); err == nil {
		t.Fatal("invalid message trigger was accepted")
	}
}

func TestCloneAgentMessageCopiesDeliveryTime(t *testing.T) {
	deliveredAt := time.Now().UTC()
	message := AgentMessage{DeliveredAt: &deliveredAt}
	clone := CloneAgentMessage(message)
	if clone.DeliveredAt == message.DeliveredAt {
		t.Fatal("clone shares delivery timestamp pointer")
	}
	if !clone.DeliveredAt.Equal(deliveredAt) {
		t.Fatalf("clone delivery timestamp = %v, want %v", clone.DeliveredAt, deliveredAt)
	}
}

func TestAgentMessageSameRequestTreatsLegacyTriggerAsQueue(t *testing.T) {
	legacy := AgentMessage{ID: "message-1", SourceTaskID: "task", AuthorAgentID: "parent", RecipientAgentID: "child", Kind: AgentMessageKindInterAgent, Content: "continue"}
	current := legacy
	current.Trigger = AgentMessageTriggerQueue
	if !legacy.SameRequest(current) {
		t.Fatal("legacy empty trigger should compare equal to queue")
	}
}
