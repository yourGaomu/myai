package runtime

import (
	"context"
	"strings"
	"testing"

	domainmessage "myai/core/domain/message"
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
	if prompt != "Skill instruction" {
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
	if prompt != "Skill instruction" {
		t.Fatalf("unexpected prompt: %q", prompt)
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

func TestInsertRuntimeInstructionsBeforeLatestHumanMessage(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "stable system"),
		domainmessage.Text(domainmessage.RoleUser, "first"),
		domainmessage.Text(domainmessage.RoleAssistant, "reply"),
		domainmessage.Text(domainmessage.RoleUser, "latest"),
	}

	withRuntime := InsertRuntimeInstructions(messages, "turn prompt")

	if len(withRuntime) != len(messages)+1 {
		t.Fatalf("expected runtime message to be inserted, got %d", len(withRuntime))
	}
	if withRuntime[3].Role != domainmessage.RoleSystem {
		t.Fatalf("expected runtime message at index 3, got %s", withRuntime[3].Role)
	}
	if withRuntime[4].Role != domainmessage.RoleUser {
		t.Fatalf("expected latest human message after runtime prompt, got %s", withRuntime[4].Role)
	}
}
