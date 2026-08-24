package session

import (
	"strings"

	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
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
	CompactionSourceHash string
	CompactionCheckpoint *compaction.Checkpoint
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
		state.CompactionSourceHash,
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
	if state.CompactionCheckpoint != nil {
		current.CompactionCheckpoint = state.CompactionCheckpoint.Clone()
		if current.Summary == "" {
			current.Summary = current.CompactionCheckpoint.Summary
		}
		if current.CompactedMessages == 0 {
			current.CompactedMessages = current.CompactionCheckpoint.SourceEndMessage
		}
		if current.CompactionSourceHash == "" {
			current.CompactionSourceHash = current.CompactionCheckpoint.SourceHistoryHash
		}
	}
	if !hadMessages {
		current.Messages = defaultMessages(state.SystemInstruction)
	}
	if current.CompactionCheckpoint != nil && !contextmgr.CompactionCheckpointMatchesCheckpoint(current.Messages, current.CompactionCheckpoint) {
		// A persisted structured checkpoint has the same fail-open policy as the
		// legacy fields: preserve messages and discard only the stale summary.
		current.Summary = ""
		current.CompactedMessages = 0
		current.CompactionSourceHash = ""
		current.CompactionCheckpoint = nil
	} else if strings.TrimSpace(current.Summary) != "" && strings.TrimSpace(current.CompactionSourceHash) != "" {
		actualHash := contextmgr.CompactionSourceHash(current.Messages, current.CompactedMessages)
		if actualHash != current.CompactionSourceHash {
			// Old summaries must never be applied to a changed message prefix.
			current.Summary = ""
			current.CompactedMessages = 0
			current.CompactionSourceHash = ""
		}
	}
	return current
}
