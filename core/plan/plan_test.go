package plan

import (
	"strings"
	"testing"
	"time"
)

func TestExtractStepsFromPlanSection(t *testing.T) {
	content := `我会先检查项目。

## Plan
1. Read project structure - locate the entry points
2. Update session storage: persist the new fields
3. Run tests

## Verification
- go test ./...`

	steps := ExtractSteps(content)
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].Title != "Read project structure" {
		t.Fatalf("unexpected first title: %q", steps[0].Title)
	}
	if steps[1].Description != "persist the new fields" {
		t.Fatalf("unexpected second description: %q", steps[1].Description)
	}
	if steps[2].Status != StepStatusPending {
		t.Fatalf("unexpected step status: %q", steps[2].Status)
	}
}

func TestExtractStepsFromChinesePlanSection(t *testing.T) {
	content := `下面先给出执行规划。

## 执行规划
1. 确定主题：选择秋夜、月色与乡思
2. 选择体式：写成七言绝句
3. 完成正文：保持古典意象与含蓄收束

## 正文
《秋夜》
月落疏窗静有声，风吹桂影入银屏。`

	steps := ExtractSteps(content)
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].Title != "确定主题" {
		t.Fatalf("unexpected first title: %q", steps[0].Title)
	}
	if steps[0].Description != "选择秋夜、月色与乡思" {
		t.Fatalf("unexpected first description: %q", steps[0].Description)
	}
}

func TestHasResultSection(t *testing.T) {
	for _, content := range []string{
		"## Result\nA complete text answer.",
		"## 最终结果\n《秋夜》",
	} {
		if !HasResultSection(content) {
			t.Fatalf("expected a standalone nonempty Result: %q", content)
		}
	}
	for _, content := range []string{
		"## Plan\n1. Draft\n\n## Result\nDone",
		"## 执行规划\n1. 写诗\n\n## 正文\n《秋夜》",
		"## Result\nDone\n\n## Plan\n1. Inspect",
		"## Plan\n1. Inspect only",
		"## Result format\nUse JSON",
		"## Output\nExpected output",
		"## Answer\nAn answer",
		"## Result\n\n## Notes\nUnrelated text",
		"```markdown\n## Result\nExample only\n```",
	} {
		if HasResultSection(content) {
			t.Fatalf("a plan, example, empty or unrelated heading must not signal completion: %q", content)
		}
	}
}

func TestExtractStepsRequiresAnExplicitPlanSection(t *testing.T) {
	for _, content := range []string{
		"Notes:\n- assumption\n- risk",
		"1. Explanation\n2. More explanation",
		"## Output\n- expected output",
		"```markdown\n## Plan\n1. example\n```",
		"An example object: {\"steps\":[{\"title\":\"example\"}]}",
	} {
		if steps := ExtractSteps(content); len(steps) != 0 {
			t.Fatalf("ordinary prose must not become executable steps: %#v", steps)
		}
	}
}

func TestExtractStepsKeepsOnlyTheFirstListLevel(t *testing.T) {
	content := "## Plan\n1. Inspect\n   - detail\n   - another detail\n2. Verify\n\nRisks:\n- assumption\n- risk"
	steps := ExtractSteps(content)
	if len(steps) != 2 || steps[0].Title != "Inspect" || steps[1].Title != "Verify" {
		t.Fatalf("nested details and following prose became steps: %#v", steps)
	}
}

func TestExtractStructuredStepsIgnoreExecutionState(t *testing.T) {
	for _, status := range []string{"done", "skipped", "running", "failed", "canceled", "unknown"} {
		t.Run(status, func(t *testing.T) {
			content := `{"status":"done","steps":[{"id":"work","title":"Work","status":"` + status + `","retry_count":5,"last_error":"old","agent_task_id":"old","started_at":"old","completed_at":"old"}]}`
			steps := ExtractSteps(content)
			if len(steps) != 1 || steps[0].Status != StepStatusPending || steps[0].RetryCount != 0 || steps[0].LastError != "" || steps[0].AgentTaskID != "" || steps[0].StartedAt != nil || steps[0].CompletedAt != nil {
				t.Fatalf("model supplied execution state was accepted: %#v", steps)
			}
		})
	}
}

func TestExtractStructuredStepsPreservesInvalidGraphForValidation(t *testing.T) {
	for _, content := range []string{
		`{"steps":[{"id":"work","title":"Work","order":0}]}`,
		`{"steps":[{"id":"work","title":"Work","depends_on":[" "]}]}`,
		`{"steps":[{"id":"work","title":"Work","depends_on":["missing"]}]}`,
		`{"steps":[{"id":"work","title":"Work","depends_on":["work"]}]}`,
	} {
		steps := ExtractSteps(content)
		if len(steps) != 1 || Validate(&Plan{Steps: steps}) == nil {
			t.Fatalf("parsing silently repaired invalid graph input: %#v", steps)
		}
	}
}

func TestExtractStructuredStepsAcceptsJSONFenceAndRejectsPartialPlans(t *testing.T) {
	steps := ExtractSteps("Intro {example}\n```json\n{\"steps\":[{\"title\":\"Work\"}]}\n```\nNotes {other}")
	if len(steps) != 1 || steps[0].Title != "Work" {
		t.Fatalf("expected fenced JSON compatibility: %#v", steps)
	}
	for _, content := range []string{
		`{"steps":[{"title":"Work"},{"title":""}]}`,
		"```json\n{\"steps\":[{\"title\":\"Work\"},]}\n```\n## Plan\n1. Do only a fragment",
		"## Plan\n" + strings.Repeat("1. Work\n", 13),
		`{"steps":[` + strings.Repeat(`{"title":"Work"},`, 12) + `{"title":"Last"}]}`,
	} {
		if steps := ExtractSteps(content); len(steps) != 0 {
			t.Fatalf("invalid or oversized plan was partially accepted: %#v", steps)
		}
	}
}

func TestNewDraftDoesNotInventExecutableStep(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	draft := NewDraft("session-1", "make a plan", "No numbered list yet.", now)
	if draft == nil {
		t.Fatal("expected draft plan")
	}
	if draft.Status != StatusDraft {
		t.Fatalf("unexpected status: %q", draft.Status)
	}
	if len(draft.Steps) != 0 {
		t.Fatalf("expected malformed plan to have no executable steps, got %d", len(draft.Steps))
	}
}

func TestRunningPlanCanResumeAfterRestart(t *testing.T) {
	if !IsExecutableStatus(StatusRunning) {
		t.Fatal("expected a running plan to be resumable")
	}
}

func TestExtractStepsFromStructuredJSON(t *testing.T) {
	steps := ExtractSteps(`{"goal":"ship it","steps":[{"id":"stable-step","title":"Implement","description":"edit files","max_retries":2},{"title":"Verify","status":"pending","depends_on":["stable-step","stable-step"]}]}`)
	if len(steps) != 2 {
		t.Fatalf("expected 2 structured steps, got %d", len(steps))
	}
	if steps[0].ID != "stable-step" || steps[0].MaxRetries != 2 || steps[1].Order != 2 {
		t.Fatalf("unexpected structured steps: %#v", steps)
	}
	if len(steps[1].Dependencies) != 1 || steps[1].Dependencies[0] != "stable-step" {
		t.Fatalf("unexpected step dependencies: %#v", steps[1].Dependencies)
	}
}

func TestCloneCopiesStepDependencies(t *testing.T) {
	original := &Plan{Steps: []Step{{ID: "step-2", Dependencies: []string{"step-1"}}}}
	cloned := Clone(original)
	cloned.Steps[0].Dependencies[0] = "changed"
	if original.Steps[0].Dependencies[0] != "step-1" {
		t.Fatal("clone must not share dependency slices")
	}
}
