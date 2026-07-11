package model

type ChatStreamHandler struct {
	OnReasoning  func(text string)
	OnAnswer     func(text string)
	OnToolCall   func(name string, arguments string)
	OnToolResult func(name string, arguments string, result string)
	OnToolAsk    func(request ToolPermissionRequest) bool
}
