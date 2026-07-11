package generation

import (
	"context"
	"fmt"
	"time"

	generationcommand "myai/core/application/chat/generation/command"
	runtimeservice "myai/core/application/runtime/service"
)

type UserMessagePersistence struct {
	Messages UserMessageWriter
	Async    runtimeservice.AsyncTaskService
	Timeout  time.Duration
	OnError  func(error)
}

func (p UserMessagePersistence) PersistUserMessage(command generationcommand.PersistUserMessage) {
	p.Async.Submit(func() {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout())
		defer cancel()
		if p.Messages == nil {
			return
		}
		err := p.Messages.SaveUserMessage(ctx, command)
		if err != nil && p.OnError != nil {
			p.OnError(fmt.Errorf("save user message failed: %w", err))
		}
	})
}

func (p UserMessagePersistence) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return defaultPersistenceTimeout
}
