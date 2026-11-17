package plan

import (
	"testing"
	"time"

	agentplan "myai/core/plan"
)

func TestCaptureServiceMarksContentOnlyPlanDone(t *testing.T) {
	content := "## Result\nA completed poem."

	captured := CaptureService{}.Capture("session-1", "write poem", content, time.Now())

	if captured.Status != agentplan.StatusDone {
		t.Fatalf("expected done plan, got %q", captured.Status)
	}
	if len(captured.Steps) != 0 {
		t.Fatalf("a standalone answer must not claim to have executed steps: %#v", captured.Steps)
	}
}

func TestCaptureServiceDoesNotTrustModelCompletion(t *testing.T) {
	for _, content := range []string{
		"## Plan\n1. Inspect files\n\n## Result\nCompleted",
		"## Plan\n1. Inspect files\n\n## Output\nExpected logs",
		"## Output\nExpected logs\n\n## Plan\n1. Inspect files",
		`{"status":"done","steps":[{"id":"inspect","title":"Inspect files","status":"skipped"}]}`,
	} {
		captured := CaptureService{}.Capture("session-1", "inspect", content, time.Now())
		if captured.Status != agentplan.StatusDraft || len(captured.Steps) != 1 || captured.Steps[0].Status != agentplan.StepStatusPending {
			t.Fatalf("model output marked unexecuted work complete: %#v", captured)
		}
	}
	for _, content := range []string{"## Result\n", "## Output\nExpected logs", "Notes:\n- an observation"} {
		captured := CaptureService{}.Capture("session-1", "answer", content, time.Now())
		if captured.Status != agentplan.StatusDraft || len(captured.Steps) != 0 {
			t.Fatalf("unexpected completion or executable prose: %#v", captured)
		}
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
