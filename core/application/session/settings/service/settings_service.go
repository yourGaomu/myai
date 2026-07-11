package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	settingscommand "myai/core/application/session/settings/command"
	settingsport "myai/core/application/session/settings/port"
	"myai/core/contextmgr"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type SettingsService struct {
	// 设置操作先确保目标会话加载到内存，再修改聚合根；持久化和事件由 UseCase 负责。
	Memory settingsport.MemoryStore
	Loader settingsport.SessionLoader
	Models modelport.Registry
}

var _ settingsport.SettingsService = SettingsService{}

func (s SettingsService) SwitchModel(ctx context.Context, command settingscommand.SwitchModel) (*session.Session, error) {
	modelID := strings.TrimSpace(command.ModelID)
	if modelID == "" {
		return nil, errors.New("model id is empty")
	}
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}
	if s.Models == nil {
		return nil, errors.New("llm client is nil")
	}
	if !s.Models.HasModel(modelID) {
		return nil, fmt.Errorf("model not found: %s", modelID)
	}

	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return nil, s.Memory.SwitchModel(modelID)
	}

	current, err := s.ensureInMemory(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := s.Memory.SwitchModelForSession(current.ID, modelID); err != nil {
		return nil, err
	}
	current.Model = modelID
	return current, nil
}

func (s SettingsService) SetPermissionMode(ctx context.Context, command settingscommand.SetPermissionMode) (*session.Session, error) {
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}

	permissionMode := session.PermissionMode(strings.TrimSpace(command.Mode))
	if !session.IsPermissionMode(permissionMode) {
		return nil, fmt.Errorf("unsupported permission mode: %s", command.Mode)
	}

	current, err := s.ensureInMemory(ctx, command.SessionID)
	if err != nil {
		return nil, err
	}
	if err := s.Memory.SetPermissionModeForSession(current.ID, permissionMode); err != nil {
		return nil, err
	}
	current.PermissionMode = permissionMode
	return current, nil
}

func (s SettingsService) SetAgentMode(ctx context.Context, command settingscommand.SetAgentMode) (*session.Session, error) {
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}

	agentMode := session.AgentMode(strings.TrimSpace(command.Mode))
	if !session.IsAgentMode(agentMode) {
		return nil, fmt.Errorf("unsupported agent mode: %s", command.Mode)
	}

	current, err := s.ensureInMemory(ctx, command.SessionID)
	if err != nil {
		return nil, err
	}
	if err := s.Memory.SetAgentModeForSession(current.ID, agentMode); err != nil {
		return nil, err
	}
	current.AgentMode = session.NormalizeAgentMode(agentMode)
	return current, nil
}

func (s SettingsService) SetContextWindow(ctx context.Context, command settingscommand.SetContextWindow) (*session.Session, error) {
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}
	if err := contextmgr.ValidateWindowK(command.WindowK); err != nil {
		return nil, err
	}

	current, err := s.ensureInMemory(ctx, command.SessionID)
	if err != nil {
		return nil, err
	}
	if err := s.Memory.SetContextWindowKForSession(current.ID, command.WindowK); err != nil {
		return nil, err
	}
	current.ContextWindowK = contextmgr.NormalizeWindowK(command.WindowK)
	return current, nil
}

func (s SettingsService) ensureInMemory(ctx context.Context, sessionID string) (*session.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}
	if s.Loader != nil {
		return s.Loader.Load(ctx, sessionID)
	}
	return s.Memory.GetSession(sessionID)
}
