package taskrecorder

import (
	"context"

	sqlitehistory "myai/core/adapter/persistence/sqlite/history/repository"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	"myai/core/history"
	historyport "myai/core/port/history"
)

type Factory struct {
	StoreFactory historyport.StoreFactory
}

func (f Factory) NewTaskRecorder(record generationcommand.TaskRecord) generationport.TaskRecorder {
	factory := f.StoreFactory
	if factory == nil {
		factory = sqlitehistory.Factory{}
	}
	return Recorder{
		task: history.NewTaskRecorder(history.RecordCommand{
			Title:     record.Title,
			Reason:    record.Reason,
			SessionID: record.SessionID,
			RequestID: record.RequestID,
		}, factory),
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
