package service

import (
	"errors"
	"strings"

	currentapi "myai/core/application/session/current/api"
	currentport "myai/core/application/session/current/port"
	currentresult "myai/core/application/session/current/result"
	"myai/core/contextmgr"
	agentplan "myai/core/plan"
	"myai/core/session"
)

type StateQueryService struct {
	Memory       currentport.StateMemory
	DefaultModel string
}

var _ currentapi.StateQueryService = StateQueryService{}

func (s StateQueryService) State() currentresult.State {
	state := currentresult.State{
		ModelID:        strings.TrimSpace(s.DefaultModel),
		AgentMode:      session.AgentModeChat,
		PermissionMode: session.PermissionModeAsk,
		ContextWindowK: contextmgr.DefaultWindowK,
	}
	if s.Memory == nil {
		return state
	}

	if modelID := strings.TrimSpace(s.Memory.CurrentModelId()); modelID != "" {
		state.ModelID = modelID
	}
	current, err := s.Memory.Current()
	if err != nil || current == nil {
		return state
	}

	state.SessionID = current.ID
	if modelID := strings.TrimSpace(current.Model); modelID != "" {
		state.ModelID = modelID
	}
	state.AgentMode = session.NormalizeAgentMode(current.AgentMode)
	state.PermissionMode = session.NormalizePermissionMode(current.PermissionMode)
	state.ContextWindowK = contextmgr.NormalizeWindowK(current.ContextWindowK)
	state.Usage = current.Usage
	state.LastUsage = current.LastUsage
	state.Plan = agentplan.Clone(current.CurrentPlan)
	return state
}

func (s StateQueryService) CurrentSession() (*session.Session, error) {
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}
	return s.Memory.Current()
}
