package api

import (
	"context"

	toolcommand "myai/core/application/tool/command"
	toolresult "myai/core/application/tool/result"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type ExecutionService interface {
	Execute(ctx context.Context, command toolcommand.Execution) (toolresult.Execution, error)
}

type PermissionService interface {
	Allow(command toolcommand.Permission) toolresult.PermissionDecision
}

type SelectionService interface {
	ToolsForSession(current *session.Session, forceChatMode bool) []modelport.Tool
}
