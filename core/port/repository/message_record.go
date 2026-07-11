package repository

import "time"

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleToolCall  = "tool_call"
	RoleTool      = "tool"
)

type MessageRecord struct {
	ID                 string
	SessionID          string
	Role               string
	Content            string
	Reasoning          string
	ToolCallID         string
	ToolName           string
	ToolArguments      string
	ToolError          string
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	ReasoningTokens    int
	PromptCachedTokens int
	CreatedAt          time.Time
}

type MessageHistoryMeta struct {
	SessionID            string
	MessageCount         int64
	LastMessageID        string
	LastMessageCreatedAt *time.Time
	HistoryVersion       int64
}
