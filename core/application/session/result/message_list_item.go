package result

import "time"

type MessageListItem struct {
	ID                    string
	Role                  string
	Content               string
	Reasoning             string
	ToolCallID            string
	ToolName              string
	ToolArguments         string
	ToolError             string
	ToolStatus            string
	ToolErrorCode         string
	ToolTruncated         bool
	PromptTokens          int
	CompletionTokens      int
	TotalTokens           int
	ReasoningTokens       int
	PromptCachedTokens    int
	PromptCacheReported   bool
	PromptCacheIncomplete bool
	CreatedAt             time.Time
}
