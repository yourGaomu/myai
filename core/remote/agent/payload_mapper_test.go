package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	sessionresult "myai/core/application/session/result"
	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	"myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
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

func TestRequestContextPayloadIsOptionalAndKeepsZeroCounts(t *testing.T) {
	info := contextmgr.Info{WindowK: 16}
	legacy, err := json.Marshal(contextInfoPayload(info))
	if err != nil || strings.Contains(string(legacy), `"request_context"`) {
		t.Fatalf("legacy payload changed: %s, %v", legacy, err)
	}
	info.RequestContext = &domainmessage.RequestContext{Model: "vision", WindowTokens: 15555, SystemTokens: 120, SafetyReserveTokens: 64}
	live := contextInfoPayload(info)
	query := contextStatePayload(service.ContextState{Info: info, Summary: "saved summary"})
	encoded, err := json.Marshal(query)
	if err != nil || live.RequestContext == nil || query.RequestContext == nil || *live.RequestContext != *query.RequestContext ||
		!strings.Contains(string(encoded), `"skill_tokens":0`) || !strings.Contains(string(encoded), `"window_tokens":15555`) || query.Summary != "saved summary" {
		t.Fatalf("unexpected context report: %s, %v", encoded, err)
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

func TestPromptCacheReportingMatchesLiveHistoryAndSessionPayloads(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage llm.TokenUsage
		want  string
	}{
		{"unknown", llm.TokenUsage{}.AsSample(), `{"prompt_cache_incomplete":true}`},
		{"zero", llm.TokenUsage{PromptCacheReported: true}, `{"prompt_cache_reported":true}`},
		{"partial zero", llm.TokenUsage{PromptCacheReported: true, PromptCacheIncomplete: true}, `{"prompt_cache_reported":true,"prompt_cache_incomplete":true}`},
		{"legacy hit", llm.TokenUsage{PromptCachedTokens: 9, Available: true}, `{"prompt_cached_tokens":9,"available":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := tc.usage
			history := tokenUsageRecordFromMessage(sessionresult.MessageListItem{
				PromptCachedTokens: u.PromptCachedTokens, PromptCacheReported: u.PromptCacheReported, PromptCacheIncomplete: u.PromptCacheIncomplete,
			})
			session := tokenUsageResultToPayload(&sessionresult.TokenUsage{
				PromptCachedTokens: u.PromptCachedTokens, PromptCacheReported: u.PromptCacheReported, PromptCacheIncomplete: u.PromptCacheIncomplete, Available: u.Available,
			})
			for path, payload := range map[string]any{"live": tokenUsagePayloadPtr(u), "history": history, "session": session} {
				got, err := json.Marshal(payload)
				if err != nil || string(got) != tc.want {
					t.Errorf("%s payload = %s, err=%v, want %s", path, got, err, tc.want)
				}
			}
		})
	}
}

func TestAssetSummariesPreserveUploadedAssetIdentityWithoutPublicShare(t *testing.T) {
	createdAt := time.Now().UTC()
	result := assetSummaries([]sessionresult.AssetListItem{{
		ID: "row", AssetID: "asset", BusinessID: "business", Checksum: "checksum", MaxVisits: 5,
		SessionID: "session", RequestID: "first-complete", FileName: "report.txt", Size: 3, CreatedAt: createdAt,
	}})
	if len(result) != 1 || result[0].AssetID != "asset" || result[0].BusinessID != "business" ||
		result[0].Checksum != "checksum" || result[0].MaxVisits != 5 || result[0].ShortURL != "" ||
		result[0].RequestID != "first-complete" || !result[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("uploaded asset fields lost in transport mapping: %#v", result)
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
