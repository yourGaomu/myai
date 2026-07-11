package service

import (
	"fmt"

	toolapi "myai/core/application/tool/api"
	toolcommand "myai/core/application/tool/command"
	toolresult "myai/core/application/tool/result"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

type PermissionService struct{}

var _ toolapi.PermissionService = PermissionService{}

func (PermissionService) Allow(command toolcommand.Permission) toolresult.PermissionDecision {
	// Hook 显式允许和只读工具无需询问；其余操作再根据会话权限模式决定。
	permission := tooldef.NormalizePermission(command.Permission)
	mode := session.NormalizePermissionMode(command.Mode)
	if command.HookAllowed || permission == tooldef.PermissionRead {
		return toolresult.PermissionDecision{Allowed: true}
	}
	switch mode {
	case session.PermissionModeReadonly:
		return toolresult.PermissionDecision{Message: fmt.Sprintf("permission denied: session permission mode is %s and tool %s requires %s", mode, command.Name, permission)}
	case session.PermissionModeFull:
		return toolresult.PermissionDecision{Allowed: true}
	default:
		if command.Ask == nil {
			return toolresult.PermissionDecision{Message: fmt.Sprintf("permission denied: tool %s requires %s but no permission handler is configured", command.Name, permission)}
		}
		allowed := command.Ask(toolcommand.PermissionRequest{Name: command.Name, Arguments: command.Arguments, Permission: permission, Mode: mode})
		if !allowed {
			return toolresult.PermissionDecision{Message: fmt.Sprintf("permission denied by user: tool %s requires %s", command.Name, permission)}
		}
		return toolresult.PermissionDecision{Allowed: true}
	}
}
