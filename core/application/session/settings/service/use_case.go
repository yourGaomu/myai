package service

import (
	"context"

	sessioncommand "myai/core/application/session/command"
	settingsapi "myai/core/application/session/settings/api"
	settingscommand "myai/core/application/session/settings/command"
	settingsport "myai/core/application/session/settings/port"
	"myai/core/session"
)

type UseCase struct {
	// UseCase 保证每个设置变更都经过“修改 -> 持久化 -> 发布事件”的统一顺序。
	Settings    settingsport.SettingsService
	Persistence settingsport.Persistence
	Events      settingsport.EventPublisher
}

var _ settingsapi.UseCase = UseCase{}

func (u UseCase) SwitchModel(ctx context.Context, command settingscommand.SwitchModel) error {
	current, err := u.Settings.SwitchModel(ctx, command)
	if err != nil || current == nil {
		return err
	}
	return u.saveAndPublish(ctx, current, "model")
}

func (u UseCase) SetPermissionMode(ctx context.Context, command settingscommand.SetPermissionMode) error {
	current, err := u.Settings.SetPermissionMode(ctx, command)
	if err != nil {
		return err
	}
	return u.saveAndPublish(ctx, current, "permission")
}

func (u UseCase) SetAgentMode(ctx context.Context, command settingscommand.SetAgentMode) error {
	current, err := u.Settings.SetAgentMode(ctx, command)
	if err != nil {
		return err
	}
	return u.saveAndPublish(ctx, current, "agent_mode")
}

func (u UseCase) SetContextWindow(ctx context.Context, command settingscommand.SetContextWindow) error {
	current, err := u.Settings.SetContextWindow(ctx, command)
	if err != nil {
		return err
	}
	return u.saveAndPublish(ctx, current, "context")
}

func (u UseCase) saveAndPublish(ctx context.Context, current *session.Session, reason string) error {
	if current == nil {
		return nil
	}
	if u.Persistence != nil {
		if err := u.Persistence.Save(ctx, sessioncommand.SaveSession{
			SessionID: current.ID,
			Model:     current.Model,
		}); err != nil {
			return err
		}
	}
	if u.Events != nil {
		u.Events.SessionChanged(ctx, current.ID, reason)
	}
	return nil
}
