package generation

import (
	"context"
	"fmt"
	"time"

	runtimeservice "myai/core/application/runtime/service"
	currentapi "myai/core/application/session/current/api"
	modelport "myai/core/port/model"
	"myai/core/session"
)

const defaultPersistenceTimeout = 10 * time.Second

type Persistence struct {
	Messages       AssistantMessageWriter
	CurrentSession currentapi.SessionService
	Async          runtimeservice.AsyncTaskService
	Timeout        time.Duration
	OnError        func(error)
}

func (p Persistence) PersistAssistant(current *session.Session, result modelport.ChatResult) {
	if current == nil {
		return
	}
	p.submit(func(ctx context.Context) error {
		if p.Messages == nil {
			return nil
		}
		return p.Messages.SaveAssistantMessage(ctx, current, result)
	}, "save assistant message")
}

func (p Persistence) PersistCurrentSession(sessionID string) {
	p.submit(func(ctx context.Context) error {
		return p.CurrentSession.Save(ctx, sessionID)
	}, "save current session")
}

func (p Persistence) submit(task func(context.Context) error, operation string) {
	p.Async.Submit(func() {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout())
		defer cancel()
		if err := task(ctx); err != nil && p.OnError != nil {
			p.OnError(fmt.Errorf("%s failed: %w", operation, err))
		}
	})
}

func (p Persistence) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return defaultPersistenceTimeout
}
