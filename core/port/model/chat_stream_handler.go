package model

import (
	domainagentrun "myai/core/domain/agentrun"
	domaintool "myai/core/domain/tool"
	agentplan "myai/core/plan"
)

type ToolResultEvent struct {
	Name      string
	Arguments string
	Output    domaintool.ToolOutput
}

type ChatStreamHandler struct {
	CorrelationID  string
	OnReasoning    func(text string)
	OnAnswer       func(text string)
	OnToolCall     func(name string, arguments string)
	OnToolResult   func(event ToolResultEvent)
	OnToolAsk      func(request ToolPermissionRequest) bool
	OnRunStarted   func(run domainagentrun.Run)
	OnRunEvent     func(event domainagentrun.Event)
	OnRunCompleted func(run domainagentrun.Run)
	OnPlanUpdate   func(plan *agentplan.Plan)
}
