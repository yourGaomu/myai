package api

import (
	"context"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	"myai/core/session"
)

type SnapshotService interface {
	Snapshot(current *session.Session, runtimePrompt string) contextmgr.Snapshot
	MessagesWithRuntimePrompt(messages []domainmessage.Message, runtimePrompt string) []domainmessage.Message
}

type QueryService interface {
	Info(ctx context.Context, current *session.Session) contextmgr.Info
	InfoWithRuntimePrompt(current *session.Session, runtimePrompt string) contextmgr.Info
}
