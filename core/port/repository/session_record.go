package repository

import (
	"time"

	compaction "myai/core/domain/compaction"
	generation "myai/core/domain/generation"
	agentplan "myai/core/plan"
	"myai/core/session"
)

type SessionRecord struct {
	ID                   string
	Kind                 string
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
	AgentMode            string
	PermissionMode       string
	ContextWindowK       int
	Summary              string
	CompactedMessages    int
	CompactionSourceHash string
	CompactionCheckpoint *compaction.Checkpoint
	CompactedAt          *time.Time
	Title                string
	Usage                *TokenUsageRecord
	LastUsage            *TokenUsageRecord
	CurrentPlan          *agentplan.Plan
	RAGSettings          session.RAGSettings
	GenerationSettings   generation.Settings
	StyleInstruction     string
	Deleted              bool
	DeletedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
