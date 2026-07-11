package plan

import (
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
	if !HasResultSection("## Plan\n1. Draft\n\n## Result\nDone") {
		t.Fatal("expected English result section")
	}
	if !HasResultSection("## 执行规划\n1. 写诗\n\n## 正文\n《秋夜》") {
		t.Fatal("expected Chinese body section")
	}
	if HasResultSection("## Plan\n1. Inspect only") {
		t.Fatal("did not expect result section")
	}
}

func TestNewDraftFallsBackToSingleStep(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	draft := NewDraft("session-1", "make a plan", "No numbered list yet.", now)
	if draft == nil {
		t.Fatal("expected draft plan")
	}
	if draft.Status != StatusDraft {
		t.Fatalf("unexpected status: %q", draft.Status)
	}
	if len(draft.Steps) != 1 {
		t.Fatalf("expected fallback step, got %d", len(draft.Steps))
	}
	if draft.Steps[0].Title != "No numbered list yet." {
		t.Fatalf("unexpected fallback title: %q", draft.Steps[0].Title)
	}
}
