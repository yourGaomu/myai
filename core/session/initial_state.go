package session

import (
	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	"myai/core/llm"
)

type InitialState struct {
	ID                 string
	Model              string
	AgentMode          AgentMode
	PermissionMode     PermissionMode
	ContextWindowK     int
	Summary            string
	CompactedMessages  int
	Usage              llm.TokenUsage
	LastUsage          llm.TokenUsage
	RAGSettings        RAGSettings
	GenerationSettings generation.Settings
	StyleInstruction   string
	Messages           []domainmessage.Message
}

func NewFromState(state InitialState) *Session {
	return newSession(
		state.ID,
		state.Model,
		state.AgentMode,
		state.PermissionMode,
		state.ContextWindowK,
		state.Summary,
		state.CompactedMessages,
		state.Usage,
		state.LastUsage,
		state.RAGSettings,
		state.GenerationSettings,
		state.StyleInstruction,
		state.Messages,
	)
}
