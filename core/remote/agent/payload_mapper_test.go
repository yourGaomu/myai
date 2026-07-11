package agent

import (
	"testing"

	sessionresult "myai/core/application/session/result"
	"myai/core/llm"
	agentplan "myai/core/plan"
	"myai/core/remote/protocol"
)

func TestResolveSessionIDUsesPayloadMessageAndCurrentOrder(t *testing.T) {
	cases := []struct {
		payload string
		message string
		current string
		want    string
	}{
		{payload: " payload ", message: "message", current: "current", want: "payload"},
		{message: " message ", current: "current", want: "message"},
		{current: " current ", want: "current"},
	}
	for _, test := range cases {
		if got := resolveSessionID(test.payload, test.message, test.current); got != test.want {
			t.Fatalf("resolveSessionID(%q, %q, %q)=%q, want %q", test.payload, test.message, test.current, got, test.want)
		}
	}
}

func TestSessionSummariesMapsApplicationResult(t *testing.T) {
	usage := &sessionresult.TokenUsage{TotalTokens: 12, Available: true}
	currentPlan := &agentplan.Plan{
		ID: "plan-1",
		Steps: []agentplan.Step{{
			ID:    "step-1",
			Order: 1,
			Title: "Inspect",
		}},
	}

	result := sessionSummaries([]sessionresult.SessionListItem{{
		ID:          "session-1",
		Title:       "Chat",
		Model:       "gpt-5",
		AgentMode:   "plan",
		Usage:       usage,
		CurrentPlan: currentPlan,
	}})

	if len(result) != 1 || result[0].ID != "session-1" || result[0].AgentMode != "plan" {
		t.Fatalf("unexpected session summaries: %#v", result)
	}
	if result[0].Usage == nil || result[0].Usage.TotalTokens != 12 {
		t.Fatalf("unexpected usage mapping: %#v", result[0].Usage)
	}
	if result[0].CurrentPlan == nil || len(result[0].CurrentPlan.Steps) != 1 {
		t.Fatalf("unexpected plan mapping: %#v", result[0].CurrentPlan)
	}
}

func TestTokenUsagePayloadPtrOmitsZeroValue(t *testing.T) {
	if tokenUsagePayloadPtr(llm.TokenUsage{}) != nil {
		t.Fatal("expected zero token usage to be omitted")
	}
	result := tokenUsagePayloadPtr(llm.TokenUsage{TotalTokens: 7, Available: true})
	if result == nil || result.TotalTokens != 7 {
		t.Fatalf("unexpected token usage payload: %#v", result)
	}
}

func TestLocalHistoryUpToDateChecksCountIDAndVersion(t *testing.T) {
	remote := sessionresult.MessageHistoryMeta{
		MessageCount:   2,
		LastMessageID:  "message-2",
		HistoryVersion: 3,
	}
	local := protocol.SessionHistoryMetaPayload{
		LocalMessageCount:   2,
		LocalLastMessageID:  "message-2",
		LocalHistoryVersion: 3,
	}
	if !localHistoryUpToDate(local, remote) {
		t.Fatal("expected matching history to be up to date")
	}
	local.LocalHistoryVersion = 2
	if localHistoryUpToDate(local, remote) {
		t.Fatal("expected version mismatch to require synchronization")
	}
}
