package port

import (
	"context"

	sessioncommand "myai/core/application/session/command"
	settingscommand "myai/core/application/session/settings/command"
	generation "myai/core/domain/generation"
	"myai/core/session"
)

type MemoryStore interface {
	GetSession(sessionID string) (*session.Session, error)
	SwitchModel(modelID string) error
	SwitchModelForSession(sessionID string, modelID string) error
	SetPermissionModeForSession(sessionID string, mode session.PermissionMode) error
	SetAgentModeForSession(sessionID string, mode session.AgentMode) error
	SetContextWindowKForSession(sessionID string, windowK int) error
	SetGenerationSettingsForSession(sessionID string, settings generation.Settings) error
	SetStyleInstructionForSession(sessionID string, instruction string) error
	SetRAGSettingsForSession(sessionID string, settings session.RAGSettings) error
}

type SessionLoader interface {
	Load(ctx context.Context, sessionID string) (*session.Session, error)
}

type Persistence interface {
	Save(ctx context.Context, command sessioncommand.SaveSession) error
}

type EventPublisher interface {
	SessionChanged(ctx context.Context, sessionID string, reason string)
}

type SettingsService interface {
	SwitchModel(ctx context.Context, command settingscommand.SwitchModel) (*session.Session, error)
	SetPermissionMode(ctx context.Context, command settingscommand.SetPermissionMode) (*session.Session, error)
	SetAgentMode(ctx context.Context, command settingscommand.SetAgentMode) (*session.Session, error)
	SetContextWindow(ctx context.Context, command settingscommand.SetContextWindow) (*session.Session, error)
	SetGenerationSettings(ctx context.Context, command settingscommand.SetGenerationSettings) (*session.Session, error)
	SetStyleInstruction(ctx context.Context, command settingscommand.SetStyleInstruction) (*session.Session, error)
	SetRAGSettings(ctx context.Context, command settingscommand.SetRAGSettings) (*session.Session, error)
}
