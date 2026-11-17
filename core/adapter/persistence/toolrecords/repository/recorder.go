package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	chatmessagemapper "myai/core/adapter/persistence/chatmessage/mapper"
	toolrecordsmapper "myai/core/adapter/persistence/toolrecords/mapper"
	toolrecordsport "myai/core/adapter/persistence/toolrecords/port"
	generationcommand "myai/core/application/chat/generation/command"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	outbox "myai/core/port/outbox"
	portrepository "myai/core/port/repository"
)

const defaultTimeout = 10 * time.Second

type Recorder struct {
	Persistence        toolrecordsport.Persistence
	IDs                toolrecordsport.IDGenerator
	RunAsync           toolrecordsport.AsyncRunner
	RunAsyncForSession toolrecordsport.SessionAsyncRunner
	// RunRetryForSession retains failed tasks; production binds SessionQueue.SubmitTask.
	RunRetryForSession func(sessionID string, task func() error) error
	SubmitRecords      func(context.Context, outbox.Batch) error
	Timeout            time.Duration
	OnError            func(error)
}

func (r Recorder) RecordToolExecution(ctx context.Context, command generationcommand.ToolExecutionRecord) {
	if r.SubmitRecords != nil {
		r.report(r.AcceptToolExecution(ctx, command))
		return
	}
	if len(command.Entries) == 0 && len(command.Assets) == 0 && len(command.Messages) == 0 {
		return
	}
	if r.Persistence == nil {
		return
	}

	batch := r.prepareRecords(command)
	r.run(batch.SessionID, func() error {
		// Tool records describe completed work. Caller cancellation must not
		// discard accepted records; each attempt gets its own bounded lifetime.
		saveCtx, cancel := context.WithTimeout(context.Background(), r.timeout())
		defer cancel()
		err := r.saveRecords(saveCtx, batch.Messages, batch.Assets)
		r.report(err)
		return err
	})
}

// AcceptToolExecution journals mapped records or explicitly completes a
// synchronous non-durable fallback. Completed tool effects are never rerun.
func (r Recorder) AcceptToolExecution(ctx context.Context, command generationcommand.ToolExecutionRecord) error {
	if len(command.Entries) == 0 && len(command.Assets) == 0 && len(command.Messages) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	batch := r.prepareRecords(command)
	frozen, _, err := outbox.Freeze(batch)
	if err != nil {
		return outbox.SafeError("prepare tool records", err)
	}
	if r.SubmitRecords != nil {
		return r.SubmitRecords(ctx, frozen)
	}
	if r.Persistence == nil {
		return nil
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.timeout())
	defer cancel()
	return outbox.SafeError("save tool records", r.saveRecords(saveCtx, frozen.Messages, frozen.Assets))
}

func (r Recorder) prepareRecords(command generationcommand.ToolExecutionRecord) outbox.Batch {
	entries := append([]domaintool.ExecutionEntry(nil), command.Entries...)
	assets := domaintool.CloneSharedAssets(command.Assets)
	messages := domainmessage.CloneAll(command.Messages)
	sessionID := command.SessionID
	if sessionID == "" {
		sessionID = toolSessionID(entries, assets)
	}
	// Freeze identities and fallback timestamps before queueing. A retained task
	// must retry the same records even after the caller changes its snapshots.
	mapper := toolrecordsmapper.Mapper{IDs: r.IDs}
	now := time.Now()
	var messageRecords []portrepository.MessageRecord
	if len(messages) > 0 {
		messageRecords = (chatmessagemapper.Mapper{IDs: r.IDs}).UserTurn(generationcommand.PersistUserMessage{
			SessionID: sessionID, AppendedMessages: messages,
		}, now)
	} else {
		messageRecords = mapper.MessageRecords(entries)
	}
	assetRecords := mapper.AssetRecords(assets)
	for index := range messageRecords {
		record := &messageRecords[index]
		if record.SessionID == "" {
			record.SessionID = sessionID
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = now.Add(time.Duration(index) * time.Nanosecond)
		}
		if record.Sequence <= 0 {
			record.Sequence = record.CreatedAt.UnixNano()
		}
	}
	for index := range assetRecords {
		record := &assetRecords[index]
		if record.SessionID == "" {
			record.SessionID = sessionID
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = now
		}
	}
	return outbox.Batch{Version: outbox.Version, SessionID: sessionID, Messages: messageRecords, Assets: assetRecords}
}

func (r Recorder) run(sessionID string, task func() error) {
	if r.RunRetryForSession != nil {
		if err := r.RunRetryForSession(sessionID, task); err != nil {
			r.report(fmt.Errorf("schedule tool record persistence: %w", err))
		}
		return
	}
	// Legacy runners cannot expose task failure to a queue, but still report it
	// through OnError. Keep them for adapters that do not support retries yet.
	legacyTask := func() { _ = task() }
	if r.RunAsyncForSession != nil && sessionID != "" {
		if err := r.RunAsyncForSession(sessionID, legacyTask); err != nil {
			r.report(fmt.Errorf("schedule tool record persistence: %w", err))
		}
		return
	}
	if r.RunAsync != nil {
		if err := r.RunAsync(legacyTask); err != nil {
			r.report(fmt.Errorf("schedule tool record persistence: %w", err))
		}
		return
	}
	_ = task()
}

func toolSessionID(entries []domaintool.ExecutionEntry, assets []domaintool.SharedAsset) string {
	for _, entry := range entries {
		if entry.SessionID != "" {
			return entry.SessionID
		}
	}
	for _, asset := range assets {
		if asset.SessionID != "" {
			return asset.SessionID
		}
	}
	return ""
}

func (r Recorder) saveRecords(ctx context.Context, messageRecords []portrepository.MessageRecord, assetRecords []portrepository.AssetRecord) error {
	var errs []error
	for _, record := range messageRecords {
		if err := r.Persistence.SaveMessage(ctx, record); err != nil {
			errs = append(errs, fmt.Errorf("save tool message records: %w", err))
		}
	}
	for _, record := range assetRecords {
		if err := r.Persistence.SaveAsset(ctx, record); err != nil {
			errs = append(errs, fmt.Errorf("save tool asset records: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (r Recorder) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return defaultTimeout
}

func (r Recorder) report(err error) {
	if err == nil || r.OnError == nil {
		return
	}
	r.OnError(err)
}
