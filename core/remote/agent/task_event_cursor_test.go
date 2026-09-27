package agent

import (
	"encoding/json"
	"testing"

	"myai/core/remote/protocol"
)

func TestAgentTaskEventCursorOnlyMovesForward(t *testing.T) {
	agent := &Agent{}
	agent.advanceTaskEventSequence(7)
	agent.advanceTaskEventSequence(3)
	agent.advanceTaskEventSequence(11)

	if got := agent.lastTaskEventSequence.Load(); got != 11 {
		t.Fatalf("expected cursor 11, got %d", got)
	}
}

func TestAgentOnlinePayloadCarriesTaskEventCursor(t *testing.T) {
	payload, err := json.Marshal(protocol.AgentOnlinePayload{
		Status: "online", BindCode: "123456", LastTaskEventSequence: 42,
	})
	if err != nil {
		t.Fatalf("marshal agent online payload: %v", err)
	}
	var decoded protocol.AgentOnlinePayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal agent online payload: %v", err)
	}
	if decoded.LastTaskEventSequence != 42 {
		t.Fatalf("expected task event cursor 42, got %d", decoded.LastTaskEventSequence)
	}
}
