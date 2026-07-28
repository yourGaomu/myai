package generation

import (
	"context"
	"fmt"
	"time"

	runtimeservice "myai/core/application/runtime/service"
	modelport "myai/core/port/model"
	"myai/core/session"
)

const defaultPersistenceTimeout = 10 * time.Second

type Persistence struct {
	Messages AssistantMessageWriter
	Queue    *SessionQueue
	Async    runtimeservice.AsyncTaskService
	Timeout  time.Duration
	Now      func() time.Time
	OnError  func(error)
}

func (p Persistence) PersistAssistant(current *session.Session, result modelport.ChatResult) {
	snapshot := session.Clone(current)
	if snapshot == nil {
		return
	}
	createdAt := p.now()
	result.ToolCalls = append([]modelport.ToolCall(nil), result.ToolCalls...)
	p.submit(snapshot.ID, func(ctx context.Context) error {
		if p.Messages == nil {
			return nil
		}
		return p.Messages.SaveAssistantMessage(ctx, snapshot, result, createdAt)
	}, "save assistant message")
}

func (p Persistence) submit(sessionID string, task func(context.Context) error, operation string) {
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout())
		defer cancel()
		if err := task(ctx); err != nil && p.OnError != nil {
			p.OnError(fmt.Errorf("%s failed: %w", operation, err))
		}
	}
	if p.Queue != nil {
		if err := p.Queue.Submit(sessionID, run); err != nil {
			p.report(fmt.Errorf("schedule %s: %w", operation, err))
		}
		return
	}
	if err := p.Async.Submit(run); err != nil {
		p.report(fmt.Errorf("schedule %s: %w", operation, err))
	}
}

func (p Persistence) report(err error) {
	if err != nil && p.OnError != nil {
		p.OnError(err)
	}
}

func (p Persistence) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return defaultPersistenceTimeout
}

func (p Persistence) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}
