package generation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"myai/core/session"
)

type TranscriptWriter interface {
	ReplaceSessionMessages(ctx context.Context, current *session.Session) error
}

type RegenerationPersistence struct {
	Messages TranscriptWriter
	Queue    *SessionQueue
	Timeout  time.Duration
	OnError  func(error)
}

func (p RegenerationPersistence) PersistRegeneratedSession(ctx context.Context, current *session.Session) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot := session.Clone(current)
	if snapshot == nil {
		return errors.New("regenerated session snapshot is nil")
	}
	if p.Messages == nil {
		return errors.New("regenerated transcript writer is nil")
	}
	persistenceBase := context.WithoutCancel(ctx)
	run := func() error {
		persistenceContext, cancel := context.WithTimeout(persistenceBase, p.timeout())
		defer cancel()
		return p.Messages.ReplaceSessionMessages(persistenceContext, snapshot)
	}
	var err error
	if p.Queue != nil {
		err = p.Queue.SubmitAndWait(ctx, snapshot.ID, run)
	} else {
		err = run()
	}
	if err != nil {
		wrapped := fmt.Errorf("replace regenerated session transcript failed: %w", err)
		p.report(wrapped)
		return wrapped
	}
	return nil
}

func (p RegenerationPersistence) report(err error) {
	if err != nil && p.OnError != nil {
		p.OnError(err)
	}
}

func (p RegenerationPersistence) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return defaultPersistenceTimeout
}
