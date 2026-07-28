package generation

import (
	"context"
	"fmt"
	"time"

	generationcommand "myai/core/application/chat/generation/command"
	runtimeservice "myai/core/application/runtime/service"
	"myai/core/session"
)

type UserMessagePersistence struct {
	Messages UserMessageWriter
	Queue    *SessionQueue
	Async    runtimeservice.AsyncTaskService
	Timeout  time.Duration
	Now      func() time.Time
	OnError  func(error)
}

func (p UserMessagePersistence) PersistUserMessage(command generationcommand.PersistUserMessage) {
	command.SessionSnapshot = session.Clone(command.SessionSnapshot)
	if command.CreatedAt.IsZero() {
		command.CreatedAt = p.now()
	}
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout())
		defer cancel()
		if p.Messages == nil {
			return
		}
		err := p.Messages.SaveUserMessage(ctx, command)
		if err != nil && p.OnError != nil {
			p.OnError(fmt.Errorf("save user message failed: %w", err))
		}
	}
	if p.Queue != nil {
		if err := p.Queue.Submit(command.SessionID, run); err != nil {
			p.report(fmt.Errorf("schedule user message persistence: %w", err))
		}
		return
	}
	if err := p.Async.Submit(run); err != nil {
		p.report(fmt.Errorf("schedule user message persistence: %w", err))
	}
}

func (p UserMessagePersistence) report(err error) {
	if err != nil && p.OnError != nil {
		p.OnError(err)
	}
}

func (p UserMessagePersistence) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return defaultPersistenceTimeout
}

func (p UserMessagePersistence) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}
