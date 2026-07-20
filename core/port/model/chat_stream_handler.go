package model

import domaintool "myai/core/domain/tool"

type ToolResultEvent struct {
	Name      string
	Arguments string
	Output    domaintool.ToolOutput
}

type ChatStreamHandler struct {
	OnReasoning  func(text string)
	OnAnswer     func(text string)
	OnToolCall   func(name string, arguments string)
	OnToolResult func(event ToolResultEvent)
	OnToolAsk    func(request ToolPermissionRequest) bool
}
