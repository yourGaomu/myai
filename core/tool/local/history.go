package local

import (
	"context"
	"os"
	"strings"

	"myai/core/history"
)

type historyRecorder interface {
	SnapshotPath(path string) (*history.FileSnapshot, error)
	RecordFileChange(ctx context.Context, path string, before *history.FileSnapshot, options history.RecordOptions) (string, error)
}

func openHistoryRecorder(ctx context.Context, configured historyRecorder, workspace string) (historyRecorder, func(), error) {
	if configured != nil {
		return configured, func() {}, nil
	}

	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		current, err := os.Getwd()
		if err != nil {
			return nil, nil, err
		}
		workspace = current
	}

	if task := history.TaskRecorderFromContext(ctx); task != nil {
		recorder, err := task.WorkspaceRecorder(workspace)
		if err != nil {
			return nil, nil, err
		}
		return recorder, func() {}, nil
	}

	recorder, err := history.NewRecorder(workspace)
	if err != nil {
		return nil, nil, err
	}
	return recorder, func() { _ = recorder.Close() }, nil
}
