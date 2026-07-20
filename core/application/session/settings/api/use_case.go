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
	SetGenerationSettings(ctx context.Context, command settingscommand.SetGenerationSettings) error
	SetStyleInstruction(ctx context.Context, command settingscommand.SetStyleInstruction) error
	SetRAGSettings(ctx context.Context, command settingscommand.SetRAGSettings) error
}
