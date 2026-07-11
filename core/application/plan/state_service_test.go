package plan

import (
	"testing"

	agentplan "myai/core/plan"
)

func TestStateServiceStartResetsSteps(t *testing.T) {
	currentPlan := &agentplan.Plan{
		Status: agentplan.StatusDraft,
		Steps: []agentplan.Step{
			{Status: agentplan.StepStatusDone},
			{Status: agentplan.StepStatusFailed},
		},
	}

	started := StateService{}.Start(currentPlan)

	if started.Status != agentplan.StatusRunning {
		t.Fatalf("expected running plan, got %q", started.Status)
	}
	for _, step := range started.Steps {
		if step.Status != agentplan.StepStatusPending {
			t.Fatalf("expected pending step, got %#v", started.Steps)
		}
	}
	if currentPlan.Status != agentplan.StatusDraft {
		t.Fatal("expected original plan to remain unchanged")
	}
}

func TestStateServiceStepTransitions(t *testing.T) {
	currentPlan := &agentplan.Plan{
		Status: agentplan.StatusRunning,
		Steps: []agentplan.Step{
			{Status: agentplan.StepStatusPending},
		},
	}

	running := StateService{}.MarkStepRunning(currentPlan, 0)
	if running.Steps[0].Status != agentplan.StepStatusRunning {
		t.Fatalf("expected running step, got %q", running.Steps[0].Status)
	}

	done := StateService{}.MarkStepDone(running, 0)
	if done.Steps[0].Status != agentplan.StepStatusDone {
		t.Fatalf("expected done step, got %q", done.Steps[0].Status)
	}

	failed := StateService{}.MarkStepFailed(done, 0)
	if failed.Status != agentplan.StatusFailed || failed.Steps[0].Status != agentplan.StepStatusFailed {
		t.Fatalf("expected failed plan and step, got %#v", failed)
	}
}
