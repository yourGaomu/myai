package po

import "time"

type SessionDocument struct {
	ID                   string                      `bson:"_id"`
	Kind                 string                      `bson:"kind,omitempty"`
	ParentSessionID      string                      `bson:"parent_session_id,omitempty"`
	ParentTaskID         string                      `bson:"parent_task_id,omitempty"`
	AgentDefinitionID    string                      `bson:"agent_definition_id,omitempty"`
	AgentDefinitionVer   int64                       `bson:"agent_definition_version,omitempty"`
	SystemInstruction    string                      `bson:"system_instruction,omitempty"`
	AllowedTools         []string                    `bson:"allowed_tools,omitempty"`
	EnforceToolAllowlist bool                        `bson:"enforce_tool_allowlist,omitempty"`
	WorkspaceRoot        string                      `bson:"workspace_root,omitempty"`
	WorkspaceSandboxID   string                      `bson:"workspace_sandbox_id,omitempty"`
	MaxToolRounds        int                         `bson:"max_tool_rounds,omitempty"`
	Model                string                      `bson:"model"`
	AgentMode            string                      `bson:"agent_mode,omitempty"`
	PermissionMode       string                      `bson:"permission_mode"`
	ContextWindowK       int                         `bson:"context_window_k"`
	Summary              string                      `bson:"summary,omitempty"`
	CompactedMessages    int                         `bson:"compacted_messages,omitempty"`
	CompactedAt          *time.Time                  `bson:"compacted_at,omitempty"`
	Title                string                      `bson:"title"`
	Usage                *TokenUsageDocument         `bson:"usage,omitempty"`
	LastUsage            *TokenUsageDocument         `bson:"last_usage,omitempty"`
	CurrentPlan          *PlanDocument               `bson:"current_plan,omitempty"`
	RAGSettings          *RAGSettingsDocument        `bson:"rag_settings,omitempty"`
	GenerationSettings   *GenerationSettingsDocument `bson:"generation_settings,omitempty"`
	StyleInstruction     string                      `bson:"style_instruction,omitempty"`
	Deleted              bool                        `bson:"deleted,omitempty"`
	DeletedAt            *time.Time                  `bson:"deleted_at,omitempty"`
	CreatedAt            time.Time                   `bson:"created_at"`
	UpdatedAt            time.Time                   `bson:"updated_at"`
}

type MessageDocument struct {
	ID                  string    `bson:"_id"`
	SessionID           string    `bson:"session_id"`
	Role                string    `bson:"role"`
	Content             string    `bson:"content"`
	Reasoning           string    `bson:"reasoning,omitempty"`
	ToolCallID          string    `bson:"tool_call_id,omitempty"`
	ToolName            string    `bson:"tool_name,omitempty"`
	ToolArguments       string    `bson:"tool_arguments,omitempty"`
	ToolError           string    `bson:"tool_error,omitempty"`
	ToolStatus          string    `bson:"tool_status,omitempty"`
	ToolErrorCode       string    `bson:"tool_error_code,omitempty"`
	ToolTruncated       bool      `bson:"tool_truncated,omitempty"`
	ToolPromptContent   string    `bson:"tool_prompt_content,omitempty"`
	ToolPromptError     string    `bson:"tool_prompt_error,omitempty"`
	ToolPromptTruncated bool      `bson:"tool_prompt_truncated,omitempty"`
	SyntheticReason     string    `bson:"synthetic_reason,omitempty"`
	PromptTokens        int       `bson:"prompt_tokens,omitempty"`
	CompletionTokens    int       `bson:"completion_tokens,omitempty"`
	TotalTokens         int       `bson:"total_tokens,omitempty"`
	ReasoningTokens     int       `bson:"reasoning_tokens,omitempty"`
	PromptCachedTokens  int       `bson:"prompt_cached_tokens,omitempty"`
	Sequence            int64     `bson:"sequence,omitempty"`
	CreatedAt           time.Time `bson:"created_at"`
}

type ToolCallReferenceDocument struct {
	ToolCallID string `bson:"tool_call_id"`
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
	ID                        string                      `bson:"_id"`
	Name                      string                      `bson:"name"`
	Provider                  string                      `bson:"provider"`
	Protocol                  string                      `bson:"protocol,omitempty"`
	AuthType                  string                      `bson:"auth_type,omitempty"`
	BaseURL                   string                      `bson:"base_url"`
	APIKey                    string                      `bson:"api_key"`
	ModelName                 string                      `bson:"model_name"`
	Enabled                   bool                        `bson:"enabled"`
	IsDefault                 bool                        `bson:"is_default"`
	DefaultGenerationSettings *GenerationSettingsDocument `bson:"default_generation_settings,omitempty"`
	CreatedAt                 time.Time                   `bson:"created_at"`
	UpdatedAt                 time.Time                   `bson:"updated_at"`
}

type GenerationSettingsDocument struct {
	Temperature     *float64 `bson:"temperature,omitempty"`
	TopP            *float64 `bson:"top_p,omitempty"`
	MaxOutputTokens *int     `bson:"max_output_tokens,omitempty"`
}

type RAGSettingsDocument struct {
	Mode             string   `bson:"mode"`
	KnowledgeBaseIDs []string `bson:"knowledge_base_ids,omitempty"`
	CategoryIDs      []string `bson:"category_ids,omitempty"`
	TopK             int      `bson:"top_k"`
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
