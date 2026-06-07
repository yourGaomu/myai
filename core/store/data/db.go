package data

import (
	"context"
	"time"
)

type SessionRecord struct {
	ID                string     `bson:"_id" json:"id"`
	Model             string     `bson:"model" json:"model"`
	PermissionMode    string     `bson:"permission_mode" json:"permission_mode"`
	ContextWindowK    int        `bson:"context_window_k" json:"context_window_k"`
	Summary           string     `bson:"summary,omitempty" json:"summary,omitempty"`
	CompactedMessages int        `bson:"compacted_messages,omitempty" json:"compacted_messages,omitempty"`
	CompactedAt       *time.Time `bson:"compacted_at,omitempty" json:"compacted_at,omitempty"`
	Title             string     `bson:"title" json:"title"`
	CreatedAt         time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt         time.Time  `bson:"updated_at" json:"updated_at"`
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

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleToolCall  = "tool_call"
	RoleTool      = "tool"
)

type Store interface {
	GetSession(ctx context.Context, sessionID string) (SessionRecord, error)
	SaveSession(ctx context.Context, session SessionRecord) error
	SaveModelConfig(ctx context.Context, model ModelConfig) error
	SaveMessage(ctx context.Context, message MessageRecord) error
	ClearMessages(ctx context.Context, sessionID string) error
	ListSessions(ctx context.Context) ([]SessionRecord, error)
	ListModelConfigs(ctx context.Context) ([]ModelConfig, error)
	ListMessages(ctx context.Context, sessionID string) ([]MessageRecord, error)
}
