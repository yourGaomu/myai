package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	toolrecordsmapper "myai/core/adapter/persistence/toolrecords/mapper"
	toolrecordsport "myai/core/adapter/persistence/toolrecords/port"
	generationcommand "myai/core/application/chat/generation/command"
	domaintool "myai/core/domain/tool"
)

const defaultTimeout = 10 * time.Second

type Recorder struct {
	Persistence toolrecordsport.Persistence
	IDs         toolrecordsport.IDGenerator
	RunAsync    toolrecordsport.AsyncRunner
	Timeout     time.Duration
	OnError     func(error)
}

func (r Recorder) RecordToolExecution(ctx context.Context, command generationcommand.ToolExecutionRecord) {
	if len(command.Entries) == 0 && len(command.Assets) == 0 {
		return
	}
	if r.Persistence == nil {
		return
	}

	entries := append([]domaintool.ExecutionEntry(nil), command.Entries...)
	assets := append([]domaintool.SharedAsset(nil), command.Assets...)
	r.run(func() {
		saveCtx, cancel := context.WithTimeout(context.Background(), r.timeout())
		defer cancel()

		if err := r.saveRecords(saveCtx, entries, assets); err != nil {
			r.report(err)
		}
	})
}

func (r Recorder) run(task func()) {
	if r.RunAsync != nil {
		r.RunAsync(task)
		return
	}
	go task()
}

func (r Recorder) saveRecords(ctx context.Context, entries []domaintool.ExecutionEntry, assets []domaintool.SharedAsset) error {
	mapper := toolrecordsmapper.Mapper{IDs: r.IDs}
	messageRecords := mapper.MessageRecords(entries)
	assetRecords := mapper.AssetRecords(assets)
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
