package local

import (
	"context"
	"os"
	"strings"

	sqlitehistory "myai/core/adapter/persistence/sqlite/history/repository"
	domainhistory "myai/core/domain/history"
	"myai/core/history"
	historyport "myai/core/port/history"
)

type historyRecorder interface {
	SnapshotPath(path string) (*domainhistory.FileSnapshot, error)
	RecordFileChange(ctx context.Context, path string, before *domainhistory.FileSnapshot, options history.RecordCommand) (string, error)
}

func NewWriteFileToolWithWorkspaceAndHistory(workspace string, factory historyport.StoreFactory) *WriteFileTool {
	return &WriteFileTool{workspace: workspace, historyFactory: factory}
}

func NewEditFileToolWithWorkspaceAndHistory(workspace string, factory historyport.StoreFactory) *EditFileTool {
	return &EditFileTool{workspace: workspace, historyFactory: factory}
}

func openHistoryRecorder(ctx context.Context, configured historyRecorder, workspace string, factories ...historyport.StoreFactory) (historyRecorder, func(), error) {
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

	var factory historyport.StoreFactory = sqlitehistory.Factory{}
	if len(factories) > 0 && factories[0] != nil {
		factory = factories[0]
	}
	recorder, err := history.NewRecorder(workspace, factory)
	if err != nil {
		return nil, nil, err
	}
	return recorder, func() { _ = recorder.Close() }, nil
}
