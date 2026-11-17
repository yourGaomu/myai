package generation

import (
	"context"
	"time"

	generationcommand "myai/core/application/chat/generation/command"
	runtimeservice "myai/core/application/runtime/service"
	domainmessage "myai/core/domain/message"
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
	if p.Queue.Durable() {
		p.report(p.AcceptUserMessage(context.Background(), command))
		return
	}
	command.SessionSnapshot = session.Clone(command.SessionSnapshot)
	command.AppendedMessages = domainmessage.CloneAll(command.AppendedMessages)
	if command.CreatedAt.IsZero() {
		command.CreatedAt = p.now()
	}
	persistence := Persistence{Queue: p.Queue, Async: p.Async, Timeout: p.Timeout, OnError: p.OnError}
	persistence.submit(command.SessionID, func(ctx context.Context) error {
		if p.Messages == nil {
			return nil
		}
		return p.Messages.SaveUserMessage(ctx, command)
	}, "save user message")
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
