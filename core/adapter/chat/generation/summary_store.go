package generation

import (
	"context"
	"errors"

	sessioncommand "myai/core/application/session/command"
	"myai/core/session"
)

type SummaryMemory interface {
	SetSummaryForSession(sessionID string, summary string, compactedMessages int) error
}

type SummaryStore struct {
	Memory   SummaryMemory
	Sessions SummaryPersistence
}

func (s SummaryStore) SaveSummary(ctx context.Context, current *session.Session, summary string, compactedMessages int) error {
	if current == nil {
		return errors.New("session is nil")
	}
	if s.Memory == nil {
		return errors.New("session manager is nil")
	}
	if err := s.Memory.SetSummaryForSession(current.ID, summary, compactedMessages); err != nil {
		return err
	}
	if s.Sessions == nil {
		return nil
	}
	return s.Sessions.Save(ctx, sessioncommand.SaveSession{
		SessionID: current.ID,
		Model:     current.Model,
	})
}
