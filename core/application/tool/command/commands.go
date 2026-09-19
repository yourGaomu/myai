package command

import (
	"time"

	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
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
	OnToolResult func(name string, arguments string, output domaintool.ToolOutput)
	OnToolAsk    PermissionAskFunc
}

type Execution struct {
	SessionID            string
	AgentMode            session.AgentMode
	PermissionMode       session.PermissionMode
	ForceChatMode        bool
	RequestID            string
	WorkspaceRoot        string
	Isolated             bool
	Calls                []domainmessage.ToolCall
	AllowedTools         []string
	EnforceToolAllowlist bool
	Callbacks            ExecutionCallbacks
}

type Permission struct {
	Name                string
	Arguments           string
	Permission          tooldef.Permission
	Mode                session.PermissionMode
	WorkspaceRoot       string
	Isolated            bool
	RequireConfirmation bool
	Ask                 PermissionAskFunc
}

type AssetExtraction struct {
	SessionID string
	RequestID string
	Call      domainmessage.ToolCall
	Output    domaintool.ToolOutput
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
	Output    domaintool.ToolOutput
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
