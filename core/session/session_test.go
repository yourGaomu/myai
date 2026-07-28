package session

import (
	"strings"
	"testing"
)

func TestSubagentInstructionUsesDedicatedSystemPrompt(t *testing.T) {
	prompt := SystemPromptWithInstruction("Inspect session handling.")
	if !strings.Contains(prompt, "background subagent") {
		t.Fatalf("subagent base prompt is missing: %q", prompt)
	}
	if !strings.Contains(prompt, "Analysis, research, explanation, and review are concrete tasks") {
		t.Fatalf("subagent execution contract is missing: %q", prompt)
	}
	if !strings.Contains(prompt, "Inspect session handling.") {
		t.Fatalf("role instruction is missing: %q", prompt)
	}
	if strings.Contains(prompt, "You are myai, a local AI coding assistant") {
		t.Fatalf("subagent must not inherit the user-session base prompt: %q", prompt)
	}
}

func TestUserSessionKeepsCodingAssistantSystemPrompt(t *testing.T) {
	if prompt := SystemPrompt(); !strings.Contains(prompt, "local AI coding assistant") {
		t.Fatalf("unexpected user-session prompt: %q", prompt)
	}
}
