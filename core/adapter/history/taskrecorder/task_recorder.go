package taskrecorder

import (
	"context"

	sqlitehistory "myai/core/adapter/persistence/sqlite/history"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	"myai/core/history"
)

type Factory struct{}

func (Factory) NewTaskRecorder(record generationcommand.TaskRecord) generationport.TaskRecorder {
	return Recorder{
		task: history.NewTaskRecorder(history.RecordCommand{
			Title:     record.Title,
			Reason:    record.Reason,
			SessionID: record.SessionID,
			RequestID: record.RequestID,
		}, sqlitehistory.Factory{}),
	}
}

type Recorder struct {
	task *history.TaskRecorder
}

func (r Recorder) Attach(ctx context.Context) context.Context {
	return history.WithTaskRecorder(ctx, r.task)
}

func (r Recorder) Save(ctx context.Context) error {
	_, err := r.task.Save(ctx)
	return err
}

func (r Recorder) Close() error {
	return r.task.Close()
}
