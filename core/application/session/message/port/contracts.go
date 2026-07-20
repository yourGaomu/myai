package port

import (
	"context"

	"myai/core/session"
)

type SessionLoader interface {
	Load(ctx context.Context, sessionID string) (*session.Session, error)
}

type CommandMemory interface {
	CurrentSessionId() string
	AddUserTurnTo(sessionID string, runtimeInstruction string, input string) error
	AddUserTurnWithContextTo(sessionID string, ragContext string, runtimeInstruction string, input string) error
	TrimAfterLastUserMessage(sessionID string) (string, error)
	GetSession(sessionID string) (*session.Session, error)
}

type RuntimeInstructionProvider interface {
	Prompt(ctx context.Context, current *session.Session, input string, forceChatMode bool) string
}
