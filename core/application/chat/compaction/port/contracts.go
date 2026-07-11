package port

import (
	"context"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type SummaryGenerator interface {
	Summarize(ctx context.Context, model modelport.ChatModelPort, existingSummary string, messages []domainmessage.Message) (string, error)
}

type SummaryStore interface {
	SaveSummary(ctx context.Context, current *session.Session, summary string, compactedMessages int) error
}

type SessionLoader interface {
	Load(ctx context.Context, sessionID string) (*session.Session, error)
}

type SessionCompactor interface {
	CompactSession(ctx context.Context, current *session.Session, model modelport.ChatModelPort) error
}

type ContextQuery interface {
	Info(ctx context.Context, current *session.Session) contextmgr.Info
}
