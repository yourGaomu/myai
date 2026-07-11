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
	agentMode := session.AgentModeChat
	if current != nil {
		agentMode = current.AgentMode
	}
	return p.Builder.Build(ctx, runtimecommand.InstructionRequest{
		AgentMode:     agentMode,
		ForceChatMode: forceChatMode,
		Input:         input,
	})
}
