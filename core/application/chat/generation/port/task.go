package port

import (
	"context"

	generationcommand "myai/core/application/chat/generation/command"
)

type RequestIDGenerator interface {
	NewRequestID() string
}

type TaskRecorderFactory interface {
	NewTaskRecorder(record generationcommand.TaskRecord) TaskRecorder
}

type TaskRecorder interface {
	Attach(ctx context.Context) context.Context
	Save(ctx context.Context) error
	Close() error
}
