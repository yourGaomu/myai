package port

import (
	"context"

	"myai/core/contextmgr"
	"myai/core/session"
)

type Provider interface {
	Snapshot(current *session.Session, runtimePrompt string) contextmgr.Snapshot
}

type RuntimeInstructionProvider interface {
	Prompt(ctx context.Context, current *session.Session, userInput string, forceChatMode bool) string
}
