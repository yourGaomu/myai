package model

type ChatResult struct {
	Content   string
	Reasoning string
	Usage     TokenUsage
	ToolCalls []ToolCall
}
