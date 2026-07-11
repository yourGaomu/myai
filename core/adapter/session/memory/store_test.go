package memory

import (
	"testing"

	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	agentplan "myai/core/plan"
	domainsession "myai/core/session"
)

func TestStoreCreatesAndUpdatesCurrentSession(t *testing.T) {
	store := NewStore("gpt-default")
	if err := store.NewSession(); err != nil {
		t.Fatal(err)
	}
	current, err := store.Current()
	if err != nil {
		t.Fatal(err)
	}
	if current.ID == "" || current.Model != "gpt-default" {
		t.Fatalf("unexpected current session: %#v", current)
	}
	if err := store.AddUserMessageTo(current.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddAssistantMessageTo(current.ID, "answer"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsageTo(current.ID, llm.TokenUsage{TotalTokens: 9}); err != nil {
		t.Fatal(err)
	}
	if len(current.Messages) != 3 || current.Usage.TotalTokens != 9 {
		t.Fatalf("unexpected updated session: %#v", current)
	}
}

func TestStoreHydratesWithoutChangingCurrentSession(t *testing.T) {
	store := NewStore("gpt-default")
	if err := store.PutSessionWithModeUsage(
		"current",
		"gpt-current",
		domainsession.AgentModeChat,
		domainsession.PermissionModeAsk,
		8,
		"",
		0,
		llm.TokenUsage{},
		llm.TokenUsage{},
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSessionWithModeUsageNoCurrent(
		"other",
		"gpt-other",
		domainsession.AgentModePlan,
		domainsession.PermissionModeReadonly,
		16,
		"summary",
		1,
		llm.TokenUsage{TotalTokens: 12},
		llm.TokenUsage{TotalTokens: 4},
		[]domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "hello")},
	); err != nil {
		t.Fatal(err)
	}
	if store.CurrentSessionId() != "current" {
		t.Fatalf("expected current session to remain unchanged, got %q", store.CurrentSessionId())
	}
	other, err := store.GetSession("other")
	if err != nil {
		t.Fatal(err)
	}
	if other.AgentMode != domainsession.AgentModePlan || other.Summary != "summary" {
		t.Fatalf("unexpected hydrated session: %#v", other)
	}
}

func TestStoreClonesCurrentPlan(t *testing.T) {
	store := NewStore("gpt-default")
	if err := store.PutSession("session-1", "gpt-default", nil); err != nil {
		t.Fatal(err)
	}
	currentPlan := &agentplan.Plan{
		ID:    "plan-1",
		Steps: []agentplan.Step{{ID: "step-1"}},
	}
	if err := store.SetCurrentPlanForSession("session-1", currentPlan); err != nil {
		t.Fatal(err)
	}
	currentPlan.Steps[0].ID = "changed"

	stored, err := store.GetSession("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.CurrentPlan.Steps[0].ID != "step-1" {
		t.Fatal("expected memory store to clone plan input")
	}
}
