package command

import (
	"time"

	domainmessage "myai/core/domain/message"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

type PermissionRequest struct {
	Name       string
	Arguments  string
	Permission tooldef.Permission
	Mode       session.PermissionMode
}

type PermissionAskFunc func(PermissionRequest) bool

type ExecutionCallbacks struct {
	OnToolCall   func(name string, arguments string)
	OnToolResult func(name string, arguments string, result string)
	OnToolAsk    PermissionAskFunc
}

type Execution struct {
	SessionID      string
	PermissionMode session.PermissionMode
	RequestID      string
	Calls          []domainmessage.ToolCall
	Callbacks      ExecutionCallbacks
}

type Permission struct {
	Name        string
	Arguments   string
	Permission  tooldef.Permission
	Mode        session.PermissionMode
	HookAllowed bool
	Ask         PermissionAskFunc
}

type AssetExtraction struct {
	SessionID string
	RequestID string
	Call      domainmessage.ToolCall
	Result    string
	CreatedAt time.Time
}

type ToolCallEntry struct {
	SessionID string
	Call      domainmessage.ToolCall
	CreatedAt time.Time
}

type ToolResultEntry struct {
	SessionID string
	Call      domainmessage.ToolCall
	Result    string
	ToolError string
	CreatedAt time.Time
}

type HookEvent struct {
	SessionID  string
	Name       string
	Arguments  string
	Permission tooldef.Permission
	Result     string
	Err        error
}
