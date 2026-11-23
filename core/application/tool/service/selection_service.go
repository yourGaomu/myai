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
	policy := s.PolicyForSession(current, forceChatMode)
	tools := s.Catalog.LLMToolsByPermission(func(permission tooldef.Permission) bool {
		return policy.permissionReason(permission) == ""
	})
	if policy.knowledgeSearch && policy.allowlist == nil {
		return tools
	}
	filtered := make([]modelport.Tool, 0, len(tools))
	for _, candidate := range tools {
		if candidate.Function == nil {
			if policy.allowlist == nil {
				filtered = append(filtered, candidate)
			}
			continue
		}
		if policy.nameReason(candidate.Function.Name) == "" {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

// SelectionPolicy answers model visibility without projecting model schemas or executing tools.
type SelectionPolicy struct {
	modePolicy      toolport.ToolModePolicy
	permissionMode  session.PermissionMode
	agentMode       session.AgentMode
	forceChatMode   bool
	knowledgeSearch bool
	allowlist       map[string]struct{}
	ceiling         []tooldef.Permission
}

func (s SelectionService) PolicyForSession(current *session.Session, forceChatMode bool) SelectionPolicy {
	policy := SelectionPolicy{
		modePolicy: s.ModePolicy, forceChatMode: forceChatMode,
		permissionMode: session.PermissionModeAsk, agentMode: session.AgentModeChat,
	}
	if current != nil {
		policy.permissionMode = session.NormalizePermissionMode(current.PermissionMode)
		policy.agentMode = session.NormalizeAgentMode(current.AgentMode)
		policy.knowledgeSearch = session.NormalizeRAGSettings(current.RAGSettings).Mode == session.RetrievalModeAuto
		policy.ceiling = tooldef.ClonePermissionCeiling(current.PermissionCeiling)
		if current.EnforceToolAllowlist || current.AllowedTools != nil {
			policy.allowlist = make(map[string]struct{}, len(current.AllowedTools))
			for _, name := range current.AllowedTools {
				policy.allowlist[name] = struct{}{}
			}
		}
	}
	return policy
}

func (p SelectionPolicy) permissionReason(permission tooldef.Permission) string {
	// Plan 生成阶段由 ModePolicy 禁止写操作；执行批准计划时 forceChatMode 会解除该限制。
	if p.modePolicy != nil && !p.modePolicy.AllowsToolPermission(permission, p.agentMode, p.forceChatMode) {
		return "agent_mode"
	}
	if !allowsPermissionMode(permission, p.permissionMode) {
		return "permission_mode"
	}
	if !tooldef.PermissionAllowsCeiling(permission, p.ceiling) {
		return "permission_ceiling"
	}
	return ""
}

func (p SelectionPolicy) nameReason(name string) string {
	// Knowledge retrieval is exposed only in auto mode; always mode searches before generation.
	if name == "knowledge_search" && !p.knowledgeSearch {
		return "retrieval_mode"
	}
	// ask_user 是交互能力，不授予工作区工具权限；Agent Loop 会检查本轮是否有用户交互通道。
	if p.allowlist != nil && name != "ask_user" {
		if _, ok := p.allowlist[name]; !ok {
			return "allowlist"
		}
	}
	return ""
}

func (p SelectionPolicy) Reason(name string, permission tooldef.Permission) string {
	if reason := p.permissionReason(permission); reason != "" {
		return reason
	}
	return p.nameReason(name)
}

func allowsPermissionMode(permission tooldef.Permission, mode session.PermissionMode) bool {
	permission = tooldef.NormalizePermission(permission)
	if permission == tooldef.PermissionUnknown {
		return false
	}
	mode = session.NormalizePermissionMode(mode)
	switch mode {
	case session.PermissionModeReadonly:
		return permission == tooldef.PermissionRead
	case session.PermissionModeReadWrite:
		return permission == tooldef.PermissionRead || permission == tooldef.PermissionWrite
	case session.PermissionModeExecute:
		return permission == tooldef.PermissionRead || permission == tooldef.PermissionExecute
	case session.PermissionModeFull:
		return true
	default:
		return true
	}
}
