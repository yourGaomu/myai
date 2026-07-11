package plan

import (
	"testing"

	modelport "myai/core/port/model"
)

func TestResponseCombinerAppendsTextAndUsage(t *testing.T) {
	current := modelport.ChatResult{
		Content:   "first",
		Reasoning: "think first",
		Usage:     modelport.TokenUsage{PromptTokens: 1, CompletionTokens: 2, Available: true},
	}
	next := modelport.ChatResult{
		Content:   "second",
		Reasoning: "think second",
		Usage:     modelport.TokenUsage{PromptTokens: 3, CompletionTokens: 4},
	}

	combined := ResponseCombiner{}.Combine(current, next)

	if combined.Content != "first\n\nsecond" {
		t.Fatalf("unexpected content: %q", combined.Content)
	}
	if combined.Reasoning != "think first\n\nthink second" {
		t.Fatalf("unexpected reasoning: %q", combined.Reasoning)
	}
	if combined.Usage.PromptTokens != 4 || combined.Usage.CompletionTokens != 6 || !combined.Usage.Available {
		t.Fatalf("unexpected usage: %#v", combined.Usage)
	}
}
