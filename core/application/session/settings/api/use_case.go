package api

import (
	"context"

	settingscommand "myai/core/application/session/settings/command"
)

type UseCase interface {
	SwitchModel(ctx context.Context, command settingscommand.SwitchModel) error
	SetPermissionMode(ctx context.Context, command settingscommand.SetPermissionMode) error
	SetAgentMode(ctx context.Context, command settingscommand.SetAgentMode) error
	SetContextWindow(ctx context.Context, command settingscommand.SetContextWindow) error
}
