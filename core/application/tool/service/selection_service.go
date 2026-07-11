package service

import (
	toolapi "myai/core/application/tool/api"
	toolport "myai/core/application/tool/port"
	modelport "myai/core/port/model"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

type SelectionService struct {
	// 工具列表同时受 AgentMode 和 PermissionMode 约束，模型看不到当前不可用的工具。
	Catalog    toolport.LLMToolCatalog
	ModePolicy toolport.ToolModePolicy
}

var _ toolapi.SelectionService = SelectionService{}

func (s SelectionService) ToolsForSession(current *session.Session, forceChatMode bool) []modelport.Tool {
	if s.Catalog == nil {
		return nil
	}
	permissionMode := session.PermissionModeAsk
	agentMode := session.AgentModeChat
	if current != nil {
		permissionMode = session.NormalizePermissionMode(current.PermissionMode)
		agentMode = session.NormalizeAgentMode(current.AgentMode)
	}
	return s.Catalog.LLMToolsByPermission(func(permission tooldef.Permission) bool {
		// Plan 生成阶段由 ModePolicy 禁止写操作；执行批准计划时 forceChatMode 会解除该限制。
		if s.ModePolicy != nil && !s.ModePolicy.AllowsToolPermission(permission, agentMode, forceChatMode) {
			return false
		}
		return allowsPermissionMode(permission, permissionMode)
	})
}

func allowsPermissionMode(permission tooldef.Permission, mode session.PermissionMode) bool {
	permission = tooldef.NormalizePermission(permission)
	mode = session.NormalizePermissionMode(mode)
	switch mode {
	case session.PermissionModeReadonly:
		return permission == tooldef.PermissionRead
	case session.PermissionModeFull:
		return true
	default:
		return true
	}
}
