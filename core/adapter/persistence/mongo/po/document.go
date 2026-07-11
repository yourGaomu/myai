package po

import "time"

type SessionDocument struct {
	ID                string              `bson:"_id"`
	Model             string              `bson:"model"`
	AgentMode         string              `bson:"agent_mode,omitempty"`
	PermissionMode    string              `bson:"permission_mode"`
	ContextWindowK    int                 `bson:"context_window_k"`
	Summary           string              `bson:"summary,omitempty"`
	CompactedMessages int                 `bson:"compacted_messages,omitempty"`
	CompactedAt       *time.Time          `bson:"compacted_at,omitempty"`
	Title             string              `bson:"title"`
	Usage             *TokenUsageDocument `bson:"usage,omitempty"`
	LastUsage         *TokenUsageDocument `bson:"last_usage,omitempty"`
	CurrentPlan       *PlanDocument       `bson:"current_plan,omitempty"`
	Deleted           bool                `bson:"deleted,omitempty"`
	DeletedAt         *time.Time          `bson:"deleted_at,omitempty"`
	CreatedAt         time.Time           `bson:"created_at"`
	UpdatedAt         time.Time           `bson:"updated_at"`
}

type MessageDocument struct {
	ID                 string    `bson:"_id"`
	SessionID          string    `bson:"session_id"`
	Role               string    `bson:"role"`
	Content            string    `bson:"content"`
	Reasoning          string    `bson:"reasoning,omitempty"`
	ToolCallID         string    `bson:"tool_call_id,omitempty"`
	ToolName           string    `bson:"tool_name,omitempty"`
	ToolArguments      string    `bson:"tool_arguments,omitempty"`
	ToolError          string    `bson:"tool_error,omitempty"`
	PromptTokens       int       `bson:"prompt_tokens,omitempty"`
	CompletionTokens   int       `bson:"completion_tokens,omitempty"`
	TotalTokens        int       `bson:"total_tokens,omitempty"`
	ReasoningTokens    int       `bson:"reasoning_tokens,omitempty"`
	PromptCachedTokens int       `bson:"prompt_cached_tokens,omitempty"`
	CreatedAt          time.Time `bson:"created_at"`
}

type AssetDocument struct {
	ID          string     `bson:"_id"`
	SessionID   string     `bson:"session_id"`
	RequestID   string     `bson:"request_id,omitempty"`
	ToolCallID  string     `bson:"tool_call_id,omitempty"`
	ToolName    string     `bson:"tool_name,omitempty"`
	LocalPath   string     `bson:"local_path,omitempty"`
	FileName    string     `bson:"file_name,omitempty"`
	ContentType string     `bson:"content_type,omitempty"`
	Size        int64      `bson:"size,omitempty"`
	ShortURL    string     `bson:"short_url"`
	ShortCode   string     `bson:"short_code,omitempty"`
	ExpiresAt   *time.Time `bson:"expires_at,omitempty"`
	Deleted     bool       `bson:"deleted,omitempty"`
	DeletedAt   *time.Time `bson:"deleted_at,omitempty"`
	CreatedAt   time.Time  `bson:"created_at"`
}

type ModelConfigDocument struct {
	ID        string    `bson:"_id"`
	Name      string    `bson:"name"`
	Provider  string    `bson:"provider"`
	BaseURL   string    `bson:"base_url"`
	APIKey    string    `bson:"api_key"`
	ModelName string    `bson:"model_name"`
	Enabled   bool      `bson:"enabled"`
	IsDefault bool      `bson:"is_default"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

type TokenUsageDocument struct {
	PromptTokens       int  `bson:"prompt_tokens,omitempty"`
	CompletionTokens   int  `bson:"completion_tokens,omitempty"`
	TotalTokens        int  `bson:"total_tokens,omitempty"`
	ReasoningTokens    int  `bson:"reasoning_tokens,omitempty"`
	PromptCachedTokens int  `bson:"prompt_cached_tokens,omitempty"`
	Available          bool `bson:"available,omitempty"`
}

type PlanDocument struct {
	ID         string             `bson:"id"`
	SessionID  string             `bson:"session_id"`
	Goal       string             `bson:"goal,omitempty"`
	Status     string             `bson:"status"`
	RawContent string             `bson:"raw_content,omitempty"`
	Steps      []PlanStepDocument `bson:"steps,omitempty"`
	CreatedAt  time.Time          `bson:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at"`
}

type PlanStepDocument struct {
	ID          string `bson:"id"`
	Order       int    `bson:"order"`
	Title       string `bson:"title"`
	Description string `bson:"description,omitempty"`
	Status      string `bson:"status"`
}
