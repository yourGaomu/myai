package runtime

import (
	"context"
	"strings"
	"testing"

	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

type stubSkillPromptProvider struct {
	prompt string
}

func (p stubSkillPromptProvider) PromptForInput(context.Context, string) string {
	return p.prompt
}

func TestRuntimeInstructionBuilderAddsPlanPrompt(t *testing.T) {
	builder := NewRuntimeInstructionBuilder(stubSkillPromptProvider{prompt: "Skill instruction"})

	prompt := builder.Build(context.Background(), InstructionRequest{
		AgentMode: session.AgentModePlan,
		Input:     "write a poem",
	})

	if !strings.Contains(prompt, PlanModePrompt) {
		t.Fatal("expected plan mode instructions")
	}
	if !strings.Contains(prompt, "Skill instruction") {
		t.Fatal("expected skill instructions")
	}
}

func TestRuntimeInstructionBuilderForceChatSkipsPlanPrompt(t *testing.T) {
	builder := NewRuntimeInstructionBuilder(stubSkillPromptProvider{prompt: "Skill instruction"})

	prompt := builder.Build(context.Background(), InstructionRequest{
		AgentMode:     session.AgentModePlan,
		ForceChatMode: true,
		Input:         "execute approved step",
	})

	if strings.Contains(prompt, PlanModePrompt) {
		t.Fatal("did not expect plan mode instructions when chat mode is forced")
	}
	if !strings.Contains(prompt, "Skill instruction") || !strings.Contains(prompt, RuntimeTurnBoundaryPrompt) {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestSessionPromptProviderUsesSessionAgentMode(t *testing.T) {
	provider := NewSessionPromptProvider(stubSkillPromptProvider{prompt: "Skill instruction"})

	prompt := provider.Prompt(context.Background(), &session.Session{AgentMode: session.AgentModePlan}, "write a poem", false)

	if !strings.Contains(prompt, PlanModePrompt) {
		t.Fatal("expected plan mode instructions")
	}
	if !strings.Contains(prompt, "Skill instruction") {
		t.Fatal("expected skill instructions")
	}
}

func TestSessionPromptProviderDefaultsNilSessionToChatMode(t *testing.T) {
	provider := NewSessionPromptProvider(stubSkillPromptProvider{prompt: "Skill instruction"})

	prompt := provider.Prompt(context.Background(), nil, "write a poem", false)

	if strings.Contains(prompt, PlanModePrompt) {
		t.Fatal("did not expect plan mode instructions for nil session")
	}
	if !strings.Contains(prompt, "Skill instruction") || !strings.Contains(prompt, RuntimeTurnBoundaryPrompt) {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestRuntimeInstructionBuilderAddsSessionStyleAfterModeAndSkillRules(t *testing.T) {
	builder := NewRuntimeInstructionBuilder(stubSkillPromptProvider{prompt: "Skill instruction"})
	prompt := builder.Build(context.Background(), InstructionRequest{
		AgentMode:        session.AgentModePlan,
		Input:            "write a poem",
		StyleInstruction: "Use concise Chinese.",
	})

	planIndex := strings.Index(prompt, PlanModePrompt)
	skillIndex := strings.Index(prompt, "Skill instruction")
	styleIndex := strings.Index(prompt, SessionStylePromptPrefix)
	if planIndex < 0 || skillIndex < planIndex || styleIndex < skillIndex {
		t.Fatalf("unexpected instruction order: %q", prompt)
	}
	if !strings.Contains(prompt, "Use concise Chinese.") {
		t.Fatalf("missing style instruction: %q", prompt)
	}
}

func TestModePolicyPlanModeOnlyAllowsReadTools(t *testing.T) {
	policy := ModePolicy{}

	if !policy.AllowsToolPermission(tooldef.PermissionRead, session.AgentModePlan, false) {
		t.Fatal("expected read tools to be allowed in plan mode")
	}
	if policy.AllowsToolPermission(tooldef.PermissionWrite, session.AgentModePlan, false) {
		t.Fatal("expected write tools to be blocked in plan mode")
	}
	if !policy.AllowsToolPermission(tooldef.PermissionWrite, session.AgentModePlan, true) {
		t.Fatal("expected force chat mode to allow write tools")
	}
}
