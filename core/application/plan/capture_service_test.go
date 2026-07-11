package plan

import (
	"testing"
	"time"

	agentplan "myai/core/plan"
)

func TestCaptureServiceMarksContentOnlyPlanDone(t *testing.T) {
	content := "## Plan\n1. Draft poem\n\n## Result\nDone"

	captured := CaptureService{}.Capture("session-1", "write poem", content, time.Now())

	if captured.Status != agentplan.StatusDone {
		t.Fatalf("expected done plan, got %q", captured.Status)
	}
	if len(captured.Steps) != 1 || captured.Steps[0].Status != agentplan.StepStatusDone {
		t.Fatalf("expected done step, got %#v", captured.Steps)
	}
}

func TestCaptureServiceKeepsActionPlanDraft(t *testing.T) {
	content := "## Plan\n1. Inspect files\n2. Edit code"

	captured := CaptureService{}.Capture("session-1", "refactor", content, time.Now())

	if captured.Status != agentplan.StatusDraft {
		t.Fatalf("expected draft plan, got %q", captured.Status)
	}
	if len(captured.Steps) != 2 {
		t.Fatalf("expected two steps, got %d", len(captured.Steps))
	}
}
