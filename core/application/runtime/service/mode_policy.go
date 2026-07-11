package service

import (
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

type ModePolicy struct{}

func (ModePolicy) EffectiveAgentMode(agentMode session.AgentMode, forceChatMode bool) session.AgentMode {
	if forceChatMode {
		return session.AgentModeChat
	}
	return session.NormalizeAgentMode(agentMode)
}

func (p ModePolicy) IsPlanMode(agentMode session.AgentMode, forceChatMode bool) bool {
	return p.EffectiveAgentMode(agentMode, forceChatMode) == session.AgentModePlan
}

func (p ModePolicy) AllowsToolPermission(permission tooldef.Permission, agentMode session.AgentMode, forceChatMode bool) bool {
	if p.IsPlanMode(agentMode, forceChatMode) && tooldef.NormalizePermission(permission) != tooldef.PermissionRead {
		return false
	}
	return true
}
