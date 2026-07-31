package agent

import (
	"testing"
	"time"

	agentrunresult "myai/core/application/agentrun/result"
	domainagentrun "myai/core/domain/agentrun"
)

func TestAgentRunSnapshotsPayloadMapsRunAndEvents(t *testing.T) {
	now := time.Date(2026, 7, 31, 1, 2, 3, 0, time.UTC)
	result := agentRunSnapshotsPayload([]agentrunresult.Snapshot{{
		Run:    domainagentrun.Run{ID: "run-1", RequestID: "request-1", SessionID: "session-1", Status: domainagentrun.StatusRunning, StartedAt: now},
		Events: []domainagentrun.Event{{ID: "event-1", RunID: "run-1", SessionID: "session-1", Sequence: 1, Type: domainagentrun.EventTypeReasoning, Content: "thinking", CreatedAt: now}},
	}})
	if len(result) != 1 || result[0].Run.RequestID != "request-1" || len(result[0].Events) != 1 || result[0].Events[0].Content != "thinking" {
		t.Fatalf("unexpected agent run payload: %#v", result)
	}
}
