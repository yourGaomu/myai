package agent

import (
	"strings"
	"testing"
	"time"

	sessionresult "myai/core/application/session/result"
	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	"myai/core/domain/generation"
	"myai/core/llm"
	agentplan "myai/core/plan"
	"myai/core/remote/protocol"
	"myai/core/service"
)

func TestContextStatePayloadIncludesSummary(t *testing.T) {
	checkpoint, err := compaction.NewCheckpoint(`{"current_goal":"继续","preferences":[],"constraints":[],"decisions":[],"completed_work":[],"modified_files":[],"tool_verification":[],"problems":[],"open_tasks":[],"next_steps":[],"references":[]}`, 0, 3, "source-hash", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	payload := contextStatePayload(service.ContextState{
		Info:       contextmgr.Info{WindowK: 16, HasSummary: true},
		Summary:    "saved summary",
		Checkpoint: &checkpoint,
	})
	if payload.WindowK != 16 || !payload.HasSummary || !strings.Contains(payload.Summary, "继续") || payload.Checkpoint == nil || payload.Checkpoint.SourceHistoryHash != "source-hash" {
		t.Fatalf("unexpected context state payload: %#v", payload)
	}
}

func TestSessionGenerationPreferencesPayloadPreservesInheritanceAndZero(t *testing.T) {
	temperature := 0.0
	modelTopP := 0.8
	maxTokens := 4096
	payload := sessionGenerationPreferencesPayload(" session-1 ", service.SessionPreferencesView{
		SessionOverrides: generation.Settings{Temperature: &temperature},
		ModelDefaults:    generation.Settings{TopP: &modelTopP, MaxOutputTokens: &maxTokens},
		Effective:        generation.ResolvedSettings{Temperature: 0, TopP: 0.8, MaxOutputTokens: 4096},
		StyleInstruction: "Use concise Chinese.",
	})
	if payload.SessionID != "session-1" || payload.SessionOverrides.Temperature == nil || *payload.SessionOverrides.Temperature != 0 {
		t.Fatalf("unexpected session overrides: %#v", payload)
	}
	if payload.SessionOverrides.TopP != nil || payload.ModelDefaults.TopP == nil || *payload.ModelDefaults.TopP != 0.8 {
		t.Fatalf("inheritance was not preserved: %#v", payload)
	}
	if payload.Effective.MaxOutputTokens != 4096 || payload.StyleInstruction != "Use concise Chinese." {
		t.Fatalf("unexpected effective preferences: %#v", payload)
	}
}

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

func TestSessionPlanExecuteUpdatePayloadUsesProvidedPlanSnapshot(t *testing.T) {
	currentPlan := &agentplan.Plan{
		ID: "plan-1", SessionID: "session-1", Status: agentplan.StatusRunning,
		Steps: []agentplan.Step{{ID: "step-1", Order: 1, Title: "Inspect", Status: agentplan.StepStatusRunning}},
	}

	payload := sessionPlanExecuteUpdatePayload(" session-1 ", currentPlan)
	if payload.SessionID != "session-1" {
		t.Fatalf("unexpected session id: %q", payload.SessionID)
	}
	if payload.Plan == nil || payload.Plan.Status != agentplan.StatusRunning {
		t.Fatalf("unexpected plan payload: %#v", payload.Plan)
	}
	if len(payload.Plan.Steps) != 1 || payload.Plan.Steps[0].Status != agentplan.StepStatusRunning {
		t.Fatalf("unexpected plan step payload: %#v", payload.Plan.Steps)
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
