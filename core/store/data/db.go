package data

import (
	"context"
	"time"
)

type SessionRecord struct {
	ID                string            `bson:"_id" json:"id"`
	Model             string            `bson:"model" json:"model"`
	PermissionMode    string            `bson:"permission_mode" json:"permission_mode"`
	ContextWindowK    int               `bson:"context_window_k" json:"context_window_k"`
	Summary           string            `bson:"summary,omitempty" json:"summary,omitempty"`
	CompactedMessages int               `bson:"compacted_messages,omitempty" json:"compacted_messages,omitempty"`
	CompactedAt       *time.Time        `bson:"compacted_at,omitempty" json:"compacted_at,omitempty"`
	Title             string            `bson:"title" json:"title"`
	Usage             *TokenUsageRecord `bson:"usage,omitempty" json:"usage,omitempty"`
	LastUsage         *TokenUsageRecord `bson:"last_usage,omitempty" json:"last_usage,omitempty"`
	Deleted           bool              `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedAt         *time.Time        `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt         time.Time         `bson:"created_at" json:"created_at"`
	UpdatedAt         time.Time         `bson:"updated_at" json:"updated_at"`
}

type TokenUsageRecord struct {
	PromptTokens       int  `bson:"prompt_tokens,omitempty" json:"prompt_tokens,omitempty"`
	CompletionTokens   int  `bson:"completion_tokens,omitempty" json:"completion_tokens,omitempty"`
	TotalTokens        int  `bson:"total_tokens,omitempty" json:"total_tokens,omitempty"`
	ReasoningTokens    int  `bson:"reasoning_tokens,omitempty" json:"reasoning_tokens,omitempty"`
	PromptCachedTokens int  `bson:"prompt_cached_tokens,omitempty" json:"prompt_cached_tokens,omitempty"`
	Available          bool `bson:"available,omitempty" json:"available,omitempty"`
}

type ModelConfig struct {
	ID        string    `bson:"_id" json:"id"`
	Name      string    `bson:"name" json:"name"`
	Provider  string    `bson:"provider" json:"provider"`
	BaseURL   string    `bson:"base_url" json:"-"`
	APIKey    string    `bson:"api_key" json:"-"`
	ModelName string    `bson:"model_name" json:"model_name"`
	Enabled   bool      `bson:"enabled" json:"enabled"`
	IsDefault bool      `bson:"is_default" json:"is_default"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

type MessageRecord struct {
	ID                 string    `bson:"_id" json:"id"`
	SessionID          string    `bson:"session_id" json:"session_id"`
	Role               string    `bson:"role" json:"role"`
	Content            string    `bson:"content" json:"content"`
	Reasoning          string    `bson:"reasoning,omitempty" json:"reasoning,omitempty"`
	ToolCallID         string    `bson:"tool_call_id,omitempty" json:"tool_call_id,omitempty"`
	ToolName           string    `bson:"tool_name,omitempty" json:"tool_name,omitempty"`
	ToolArguments      string    `bson:"tool_arguments,omitempty" json:"tool_arguments,omitempty"`
	ToolError          string    `bson:"tool_error,omitempty" json:"tool_error,omitempty"`
	PromptTokens       int       `bson:"prompt_tokens,omitempty" json:"prompt_tokens,omitempty"`
	CompletionTokens   int       `bson:"completion_tokens,omitempty" json:"completion_tokens,omitempty"`
	TotalTokens        int       `bson:"total_tokens,omitempty" json:"total_tokens,omitempty"`
	ReasoningTokens    int       `bson:"reasoning_tokens,omitempty" json:"reasoning_tokens,omitempty"`
	PromptCachedTokens int       `bson:"prompt_cached_tokens,omitempty" json:"prompt_cached_tokens,omitempty"`
	CreatedAt          time.Time `bson:"created_at" json:"created_at"`
}

type MessageHistoryMeta struct {
	SessionID            string     `json:"session_id"`
	MessageCount         int64      `json:"message_count"`
	LastMessageID        string     `json:"last_message_id,omitempty"`
	LastMessageCreatedAt *time.Time `json:"last_message_created_at,omitempty"`
	HistoryVersion       int64      `json:"history_version"`
}

type AssetRecord struct {
	ID          string     `bson:"_id" json:"id"`
	SessionID   string     `bson:"session_id" json:"session_id"`
	RequestID   string     `bson:"request_id,omitempty" json:"request_id,omitempty"`
	ToolCallID  string     `bson:"tool_call_id,omitempty" json:"tool_call_id,omitempty"`
	ToolName    string     `bson:"tool_name,omitempty" json:"tool_name,omitempty"`
	LocalPath   string     `bson:"local_path,omitempty" json:"path,omitempty"`
	FileName    string     `bson:"file_name,omitempty" json:"file_name,omitempty"`
	ContentType string     `bson:"content_type,omitempty" json:"content_type,omitempty"`
	Size        int64      `bson:"size,omitempty" json:"size,omitempty"`
	ShortURL    string     `bson:"short_url" json:"short_url"`
	ShortCode   string     `bson:"short_code,omitempty" json:"code,omitempty"`
	ExpiresAt   *time.Time `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
	Deleted     bool       `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedAt   *time.Time `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt   time.Time  `bson:"created_at" json:"created_at"`
}

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleToolCall  = "tool_call"
	RoleTool      = "tool"
)

type Store interface {
	GetSession(ctx context.Context, sessionID string) (SessionRecord, error)
	SaveSession(ctx context.Context, session SessionRecord) error
	MarkSessionDeleted(ctx context.Context, sessionID string, deletedAt time.Time) error
	MarkSessionRestored(ctx context.Context, sessionID string, restoredAt time.Time) error
	SaveModelConfig(ctx context.Context, model ModelConfig) error
	SaveMessage(ctx context.Context, message MessageRecord) error
	SaveAsset(ctx context.Context, asset AssetRecord) error
	ClearMessages(ctx context.Context, sessionID string) error
	ListSessions(ctx context.Context) ([]SessionRecord, error)
	ListSessionsWithDeleted(ctx context.Context, includeDeleted bool) ([]SessionRecord, error)
	ListModelConfigs(ctx context.Context) ([]ModelConfig, error)
	ListMessages(ctx context.Context, sessionID string) ([]MessageRecord, error)
	GetMessageHistoryMeta(ctx context.Context, sessionID string) (MessageHistoryMeta, error)
	ListMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]MessageRecord, bool, error)
	ListAssets(ctx context.Context, sessionID string, limit int) ([]AssetRecord, error)
}
