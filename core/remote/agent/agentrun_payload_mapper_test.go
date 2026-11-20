package agent

import (
	"encoding/json"
	"testing"
	"time"

	agentrunresult "myai/core/application/agentrun/result"
	domainagentrun "myai/core/domain/agentrun"
	"myai/core/remote/protocol"
	"myai/core/service"
)

func TestAgentRunSnapshotsPayloadMapsRunAndEvents(t *testing.T) {
	now := time.Date(2026, 7, 31, 1, 2, 3, 0, time.UTC)
	result := agentRunSnapshotsPayload([]agentrunresult.Snapshot{{
		Run: domainagentrun.Run{ID: "run-1", RequestID: "request-1", SessionID: "session-1", Status: domainagentrun.StatusSucceeded, StartedAt: now},
		Events: []domainagentrun.Event{
			{ID: "event-1", RunID: "run-1", SessionID: "session-1", Sequence: 1, Type: domainagentrun.EventTypeReasoning, Content: "thinking", CreatedAt: now},
			{ID: "event-2", RunID: "run-1", SessionID: "session-1", Sequence: 2, Type: domainagentrun.EventTypeAnswer, Content: "reply", MessageID: "history-row", CreatedAt: now},
		},
	}})
	if len(result) != 1 || result[0].Run.RequestID != "request-1" || len(result[0].Events) != 2 || result[0].Events[0].Content != "thinking" {
		t.Fatalf("unexpected agent run payload: %#v", result)
	}
	if events := result[0].Events; events[0].MessageID != "" || events[1].MessageID != "history-row" || events[1].ID != "event-2" {
		t.Fatalf("answer lost its history row or event identity: %+v", events)
	}
}

func TestAssistantReplyIDIsOptionalOnWire(t *testing.T) {
	for _, payload := range []any{
		protocol.AssistantDonePayload{Content: "plan summary"},
		protocol.AssistantDonePayload{Paused: true},
		backgroundTurnPayload(service.BackgroundTurnEvent{Kind: "completed", Status: "failed", Error: "journal rejected"}),
		agentRunEventPayload(domainagentrun.Event{Type: domainagentrun.EventTypeAnswer, Content: "legacy answer"}),
	} {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if _, exists := fields["message_id"]; exists {
			t.Fatalf("%T with no committed reply serialized message_id: %s", payload, encoded)
		}
	}
}
