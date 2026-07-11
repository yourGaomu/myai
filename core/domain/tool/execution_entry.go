package tool

import "time"

type ExecutionEntryKind string

const (
	ExecutionEntryToolCall   ExecutionEntryKind = "tool_call"
	ExecutionEntryToolResult ExecutionEntryKind = "tool_result"
)

type ExecutionEntry struct {
	Kind       ExecutionEntryKind
	SessionID  string
	ToolCallID string
	ToolName   string
	Arguments  string
	Content    string
	Error      string
	CreatedAt  time.Time
}
