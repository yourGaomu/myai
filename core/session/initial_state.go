package session

import (
	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	"myai/core/llm"
)

type InitialState struct {
	ID                   string
	Kind                 Kind
	ParentSessionID      string
	ParentTaskID         string
	AgentDefinitionID    string
	AgentDefinitionVer   int64
	SystemInstruction    string
	AllowedTools         []string
	EnforceToolAllowlist bool
	WorkspaceRoot        string
	WorkspaceSandboxID   string
	MaxToolRounds        int
	Model                string
	AgentMode            AgentMode
	PermissionMode       PermissionMode
	ContextWindowK       int
	Summary              string
	CompactedMessages    int
	Usage                llm.TokenUsage
	LastUsage            llm.TokenUsage
	RAGSettings          RAGSettings
	GenerationSettings   generation.Settings
	StyleInstruction     string
	Messages             []domainmessage.Message
}

func NewFromState(state InitialState) *Session {
	hadMessages := len(state.Messages) > 0
	current := newSession(
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
	current.Kind = NormalizeKind(state.Kind)
	current.ParentSessionID = state.ParentSessionID
	current.ParentTaskID = state.ParentTaskID
	current.AgentDefinitionID = state.AgentDefinitionID
	current.AgentDefinitionVer = state.AgentDefinitionVer
	current.SystemInstruction = state.SystemInstruction
	current.AllowedTools = append([]string(nil), state.AllowedTools...)
	current.EnforceToolAllowlist = state.EnforceToolAllowlist
	current.WorkspaceRoot = state.WorkspaceRoot
	current.WorkspaceSandboxID = state.WorkspaceSandboxID
	current.MaxToolRounds = state.MaxToolRounds
	if !hadMessages {
		current.Messages = defaultMessages(state.SystemInstruction)
	}
	return current
}
