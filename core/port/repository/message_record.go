package repository

import "time"

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleToolCall  = "tool_call"
	RoleTool      = "tool"
)

type MessageRecord struct {
	ID                  string
	SessionID           string
	Role                string
	Content             string
	Reasoning           string
	ToolCallID          string
	ToolName            string
	ToolArguments       string
	ToolError           string
	ToolStatus          string
	ToolErrorCode       string
	ToolTruncated       bool
	ToolPromptContent   string
	ToolPromptError     string
	ToolPromptTruncated bool
	SyntheticReason     string
	PromptTokens        int
	CompletionTokens    int
	TotalTokens         int
	ReasoningTokens     int
	PromptCachedTokens  int
	Sequence            int64
	CreatedAt           time.Time
}

type MessageHistoryMeta struct {
	SessionID            string
	MessageCount         int64
	LastMessageID        string
	LastMessageCreatedAt *time.Time
	HistoryVersion       int64
}
