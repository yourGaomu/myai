package generation

import (
	"context"
	"errors"
	"time"

	generationcommand "myai/core/application/chat/generation/command"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	outbox "myai/core/port/outbox"
	"myai/core/session"
)

type UserRecordPreparer interface {
	PrepareUserMessage(generationcommand.PersistUserMessage) (outbox.Batch, error)
}
type AssistantRecordPreparer interface {
	PrepareAssistantMessage(*session.Session, modelport.ChatResult, time.Time) (outbox.Batch, error)
}

// AcceptUserMessage returns only after journal acceptance (durable queue), or a
// confirmed synchronous database attempt (explicit non-durable fallback).
func (p UserMessagePersistence) AcceptUserMessage(ctx context.Context, command generationcommand.PersistUserMessage) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	command.SessionSnapshot = session.Clone(command.SessionSnapshot)
	command.AppendedMessages = domainmessage.CloneAll(command.AppendedMessages)
	if command.CreatedAt.IsZero() {
		command.CreatedAt = p.now()
	}
	if p.Queue.Durable() {
		preparer, ok := p.Messages.(UserRecordPreparer)
		if !ok {
			return errors.New("durable user message preparer is unavailable")
		}
		batch, err := preparer.PrepareUserMessage(command)
		if err != nil {
			return outbox.SafeError("prepare user records", err)
		}
		return p.Queue.SubmitRecords(ctx, batch)
	}
	if p.Messages == nil {
		return nil
	}
	run := func() error {
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.timeout())
		defer cancel()
		return outbox.SafeError("save user message", p.Messages.SaveUserMessage(saveCtx, command))
	}
	if p.Queue != nil {
		return p.Queue.SubmitAndWait(ctx, command.SessionID, run)
	}
	return run()
}

func (p Persistence) AcceptAssistant(ctx context.Context, current *session.Session, result modelport.ChatResult) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot := session.Clone(current)
	if snapshot == nil {
		return errors.New("assistant persistence session is nil")
	}
	createdAt := p.now()
	result.ToolCalls = append([]modelport.ToolCall(nil), result.ToolCalls...)
	if result.Message != nil {
		message := domainmessage.Clone(*result.Message)
		result.Message = &message
	}
	if p.Queue.Durable() {
		preparer, ok := p.Messages.(AssistantRecordPreparer)
		if !ok {
			return errors.New("durable assistant message preparer is unavailable")
		}
		batch, err := preparer.PrepareAssistantMessage(snapshot, result, createdAt)
		if err != nil {
			return outbox.SafeError("prepare assistant records", err)
		}
		return p.Queue.SubmitRecords(ctx, batch)
	}
	if p.Messages == nil {
		return nil
	}
	run := func() error {
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.timeout())
		defer cancel()
		return outbox.SafeError("save assistant message", p.Messages.SaveAssistantMessage(saveCtx, snapshot, result, createdAt))
	}
	if p.Queue != nil {
		return p.Queue.SubmitAndWait(ctx, snapshot.ID, run)
	}
	return run()
}
