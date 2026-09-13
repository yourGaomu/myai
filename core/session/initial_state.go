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
		if current.CompactionCheckpoint.SourceEndMessage > 0 {
			current.CompactedMessages = current.CompactionCheckpoint.SourceEndMessage
		}
		if current.CompactionSourceHash == "" {
			current.CompactionSourceHash = current.CompactionCheckpoint.SourceHistoryHash
		}
	}
	if !hadMessages {
		current.Messages = defaultMessages(state.SystemInstruction)
	}
	if current.CompactionCheckpoint != nil && current.CompactionCheckpoint.Version == 0 && strings.TrimSpace(current.CompactionCheckpoint.SourceHistoryHash) == "" {
		if hash := rebuildLegacyCompactionHash(current.Messages, current.CompactionCheckpoint.SourceEndMessage); hash != "" {
			current.CompactionCheckpoint.SourceHistoryHash = hash
			current.CompactionSourceHash = hash
		} else {
			current.CompactionCheckpoint = nil
		}
	}
	if current.CompactionCheckpoint != nil && !contextmgr.CompactionCheckpointMatchesCheckpoint(current.Messages, current.CompactionCheckpoint) {
		// A checkpoint that cannot be tied to the current history is discarded,
		// while the original messages remain available for a fresh compaction.
		current.Summary = ""
		current.CompactedMessages = 0
		current.CompactionSourceHash = ""
		current.CompactionCheckpoint = nil
	} else if strings.TrimSpace(current.Summary) != "" {
		if strings.TrimSpace(current.CompactionSourceHash) == "" {
			current.CompactionSourceHash = rebuildLegacyCompactionHash(current.Messages, current.CompactedMessages)
		}
		if strings.TrimSpace(current.CompactionSourceHash) == "" ||
			contextmgr.CompactionSourceHash(current.Messages, current.CompactedMessages) != current.CompactionSourceHash {
			// Legacy summaries with no reconstructable source prefix are not applied.
			current.Summary = ""
			current.CompactedMessages = 0
			current.CompactionSourceHash = ""
		}
	}
	return current
}

func rebuildLegacyCompactionHash(messages []domainmessage.Message, compactedMessages int) string {
	if len(messages) == 0 || compactedMessages <= 0 || compactedMessages > len(messages) {
		return ""
	}
	return contextmgr.CompactionSourceHash(messages, compactedMessages)
}
