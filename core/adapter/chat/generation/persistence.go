package generation

import (
	"context"
	"fmt"
	"time"

	runtimeservice "myai/core/application/runtime/service"
	domainmessage "myai/core/domain/message"
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
	if result.Message != nil {
		message := domainmessage.Clone(*result.Message)
		result.Message = &message
	}
	p.submit(snapshot.ID, func(ctx context.Context) error {
		if p.Messages == nil {
			return nil
		}
		return p.Messages.SaveAssistantMessage(ctx, snapshot, result, createdAt)
	}, "save assistant message")
}

func (p Persistence) submit(sessionID string, task func(context.Context) error, operation string) {
	run := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout())
		defer cancel()
		if err := task(ctx); err != nil {
			wrapped := fmt.Errorf("%s failed: %w", operation, err)
			p.report(wrapped)
			return wrapped
		}
		return nil
	}
	if p.Queue != nil {
		if err := p.Queue.SubmitTask(sessionID, run); err != nil {
			p.report(fmt.Errorf("schedule %s: %w", operation, err))
		}
		return
	}
	if err := p.Async.Submit(func() { _ = run() }); err != nil {
		p.report(fmt.Errorf("schedule %s: %w", operation, err))
		_ = run()
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
