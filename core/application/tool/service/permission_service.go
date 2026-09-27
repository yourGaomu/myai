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
	// Hook 决策在本服务之前处理，任何 Hook 结果都不能提升会话权限。
	permission := tooldef.NormalizePermission(command.Permission)
	mode := session.NormalizePermissionMode(command.Mode)
	switch mode {
	case session.PermissionModeReadonly:
		if permission != tooldef.PermissionRead {
			return deniedByCapability(command, mode, permission)
		}
	case session.PermissionModeReadWrite:
		if permission == tooldef.PermissionExecute {
			return deniedByCapability(command, mode, permission)
		}
	case session.PermissionModeExecute:
		if permission == tooldef.PermissionWrite {
			return deniedByCapability(command, mode, permission)
		}
	}
	if command.RequireConfirmation {
		return askForPermission(command, permission, mode, "hook requires confirmation")
	}
	if permission == tooldef.PermissionRead {
		return toolresult.PermissionDecision{Allowed: true}
	}
	switch mode {
	case session.PermissionModeReadWrite, session.PermissionModeExecute, session.PermissionModeFull:
		return toolresult.PermissionDecision{Allowed: true}
	default:
		if assessAskApproval(command) == askApprovalAuto {
			return toolresult.PermissionDecision{Allowed: true}
		}
		return askForPermission(command, permission, mode, "")
	}
}

func deniedByCapability(command toolcommand.Permission, mode session.PermissionMode, permission tooldef.Permission) toolresult.PermissionDecision {
	return toolresult.PermissionDecision{Message: fmt.Sprintf("permission denied: session permission mode is %s and tool %s requires %s", mode, command.Name, permission)}
}

func askForPermission(command toolcommand.Permission, permission tooldef.Permission, mode session.PermissionMode, reason string) toolresult.PermissionDecision {
	if command.Ask == nil {
		message := fmt.Sprintf("permission denied: tool %s requires %s but no permission handler is configured", command.Name, permission)
		if reason != "" {
			message += ": " + reason
		}
		return toolresult.PermissionDecision{Message: message}
	}
	allowed := command.Ask(toolcommand.PermissionRequest{Name: command.Name, Arguments: command.Arguments, Permission: permission, Mode: mode})
	if !allowed {
		message := fmt.Sprintf("permission denied by user: tool %s requires %s", command.Name, permission)
		if reason != "" {
			message += ": " + reason
		}
		return toolresult.PermissionDecision{Message: message}
	}
	return toolresult.PermissionDecision{Allowed: true}
}
