package service

import (
	"context"

	runtimecommand "myai/core/application/runtime/command"
	runtimeport "myai/core/application/runtime/port"
	"myai/core/session"
)

type SessionPromptProvider struct {
	Builder RuntimeInstructionBuilder
}

func NewSessionPromptProvider(skillPrompts runtimeport.SkillPromptProvider) SessionPromptProvider {
	return SessionPromptProvider{
		Builder: NewRuntimeInstructionBuilder(skillPrompts),
	}
}

func (p SessionPromptProvider) Prompt(ctx context.Context, current *session.Session, input string, forceChatMode bool) string {
	return p.PromptForMode(ctx, current, input, forceChatMode, false)
}

// PromptForMode is the optional extended prompt contract used by autonomous
// planning. Prompt remains unchanged for existing callers and integrations.
func (p SessionPromptProvider) PromptForMode(ctx context.Context, current *session.Session, input string, forceChatMode bool, forceAutonomousPlanning bool) string {
	text, _ := p.PromptForModeWithAccounting(ctx, current, input, forceChatMode, forceAutonomousPlanning)
	return text
}

func (p SessionPromptProvider) PromptForModeWithAccounting(ctx context.Context, current *session.Session, input string, forceChatMode bool, forceAutonomousPlanning bool) (text string, skillPromptTokens int) {
	agentMode := session.AgentModeChat
	styleInstruction := ""
	if current != nil {
		agentMode = current.AgentMode
		styleInstruction = current.StyleInstruction
	}
	return p.Builder.BuildWithAccounting(ctx, runtimecommand.InstructionRequest{
		AgentMode:               agentMode,
		ForceChatMode:           forceChatMode,
		ForceAutonomousPlanning: forceAutonomousPlanning,
		Input:                   input,
		StyleInstruction:        styleInstruction,
	})
}
