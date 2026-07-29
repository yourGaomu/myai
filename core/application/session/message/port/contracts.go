package port

import (
	"context"

	domainmessage "myai/core/domain/message"
	"myai/core/session"
)

type SessionLoader interface {
	Load(ctx context.Context, sessionID string) (*session.Session, error)
}

type CommandMemory interface {
	CurrentSessionId() string
	AddUserTurnTo(sessionID string, runtimeInstruction string, input string) error
	AddUserTurnWithContextTo(sessionID string, ragContext string, runtimeInstruction string, input string) error
	AddUserTurnWithReasonTo(sessionID string, ragContext string, runtimeInstruction string, input string, reason domainmessage.SyntheticReason) error
	TrimAfterLastUserMessage(sessionID string) (string, error)
	RestoreSession(snapshot *session.Session) error
	GetSession(sessionID string) (*session.Session, error)
}

type RuntimeInstructionProvider interface {
	Prompt(ctx context.Context, current *session.Session, input string, forceChatMode bool) string
}

type RegenerationPersistence interface {
	PersistRegeneratedSession(ctx context.Context, current *session.Session) error
}
