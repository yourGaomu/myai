package model

import (
	domainagentrun "myai/core/domain/agentrun"
	domaininteraction "myai/core/domain/interaction"
	domaintool "myai/core/domain/tool"
	agentplan "myai/core/plan"
)

type ToolResultEvent struct {
	ToolCallID string
	Name       string
	Arguments  string
	Output     domaintool.ToolOutput
}

type ToolCallEvent struct {
	ToolCallID string
	Name       string
	Arguments  string
}

type ChatStreamHandler struct {
	CorrelationID   string
	OnReasoning     func(text string)
	OnAnswer        func(text string)
	OnToolCall      func(name string, arguments string)
	OnToolCallEvent func(event ToolCallEvent)
	OnToolResult    func(event ToolResultEvent)
	OnAsset         func(asset domaintool.SharedAsset)
	OnToolAsk       func(request ToolPermissionRequest) bool
	OnUserQuestion  domaininteraction.UserQuestionHandler
	OnRunStarted    func(run domainagentrun.Run)
	OnRunEvent      func(event domainagentrun.Event)
	OnRunCompleted  func(run domainagentrun.Run)
	OnPlanUpdate    func(plan *agentplan.Plan)
}
