package service

import (
	"testing"

	domainmessage "myai/core/domain/message"
	"myai/core/port/repository"
)

func TestPlanStepPromptIsOmittedFromVisibleHistory(t *testing.T) {
	records := []repository.MessageRecord{
		{ID: "user", Role: repository.RoleUser, Content: "implement this"},
		{ID: "plan-step", Role: repository.RoleUser, Content: "Execute the approved plan step", SyntheticReason: string(domainmessage.SyntheticReasonPlanStep)},
		{ID: "assistant", Role: repository.RoleAssistant, Content: "done"},
	}
	visible := visibleMessageRecords(records)
	if len(visible) != 2 || visible[0].ID != "user" || visible[1].ID != "assistant" {
		t.Fatalf("plan execution prompt leaked into visible history: %#v", visible)
	}
}
