package sessionapp

import (
	"testing"

	memorysession "myai/core/adapter/session/memory"
	"myai/core/contextmgr"
	"myai/core/llm"
	agentplan "myai/core/plan"
	"myai/core/session"
)

func TestCurrentStateQueryServiceReturnsDefaultsWithoutMemory(t *testing.T) {
	state := (CurrentStateQueryService{DefaultModel: "gpt-default"}).State()

	if state.SessionID != "" || state.ModelID != "gpt-default" {
		t.Fatalf("unexpected identity defaults: %#v", state)
	}
	if state.AgentMode != session.AgentModeChat || state.PermissionMode != session.PermissionModeAsk {
		t.Fatalf("unexpected mode defaults: %#v", state)
	}
	if state.ContextWindowK != contextmgr.DefaultWindowK {
		t.Fatalf("unexpected context default: %#v", state)
	}
}

func TestCurrentStateQueryServiceBuildsNormalizedSnapshot(t *testing.T) {
	memory := memorysession.NewStore("gpt-default")
	if err := memory.PutSessionWithOptions("session-1", "gpt-5", session.PermissionModeReadonly, 8, nil); err != nil {
		t.Fatal(err)
	}
	if err := memory.SetAgentModeForSession("session-1", session.AgentModePlan); err != nil {
		t.Fatal(err)
	}
	if err := memory.AddUsageTo("session-1", llm.TokenUsage{TotalTokens: 12}); err != nil {
		t.Fatal(err)
	}
	currentPlan := &agentplan.Plan{ID: "plan-1", Steps: []agentplan.Step{{ID: "step-1"}}}
	if err := memory.SetCurrentPlanForSession("session-1", currentPlan); err != nil {
		t.Fatal(err)
	}

	state := (CurrentStateQueryService{Memory: memory, DefaultModel: "gpt-default"}).State()
	if state.SessionID != "session-1" || state.ModelID != "gpt-5" {
		t.Fatalf("unexpected identity: %#v", state)
	}
	if state.AgentMode != session.AgentModePlan || state.PermissionMode != session.PermissionModeReadonly {
		t.Fatalf("unexpected modes: %#v", state)
	}
	if state.ContextWindowK != 8 || state.Usage.TotalTokens != 12 {
		t.Fatalf("unexpected context or usage: %#v", state)
	}
	if state.Plan == nil || state.Plan.ID != "plan-1" {
		t.Fatalf("unexpected plan: %#v", state.Plan)
	}
	state.Plan.Steps[0].ID = "changed"
	if currentPlan.Steps[0].ID != "step-1" {
		t.Fatal("expected plan snapshot to be cloned")
	}
}
