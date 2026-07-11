package history

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	domainhistory "myai/core/domain/history"
	historyport "myai/core/port/history"
)

type Recorder struct {
	// Recorder 为单次文件改动保存 before/after 快照；底层 Store 当前由 SQLite 实现。
	workspace string
	store     historyport.Store
}

func NewRecorder(workspace string, factory historyport.StoreFactory) (*Recorder, error) {
	if factory == nil {
		return nil, errors.New("history store factory is nil")
	}
	workspace, err := cleanWorkspace(workspace)
	if err != nil {
		return nil, err
	}

	dbPath, err := factory.DefaultPath(workspace)
	if err != nil {
		return nil, err
	}
	store, err := factory.Open(dbPath)
	if err != nil {
		return nil, err
	}

	return &Recorder{
		workspace: workspace,
		store:     store,
	}, nil
}

func NewRecorderWithStore(workspace string, store historyport.Store) (*Recorder, error) {
	if store == nil {
		return nil, errors.New("history store is nil")
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = "."
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &Recorder{workspace: abs, store: store}, nil
}

func (r *Recorder) Close() error {
	if r == nil || r.store == nil {
		return nil
	}
	return r.store.Close()
}

func (r *Recorder) RecordFileChange(ctx context.Context, path string, before *domainhistory.FileSnapshot, command RecordCommand) (string, error) {
	if r == nil || r.store == nil {
		return "", nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	rel, abs, err := r.cleanPath(path)
	if err != nil {
		return "", err
	}
	after, exists, err := SnapshotFile(abs, rel)
	if err != nil {
		return "", err
	}

	var afterPtr *domainhistory.FileSnapshot
	if exists {
		afterPtr = &after
	}
	// 内容未变化时不创建空检查点，避免历史列表被无效操作污染。
	if !snapshotChanged(before, afterPtr) {
		return "", nil
	}

	change := domainhistory.FileChange{
		Path:       rel,
		ChangeType: changeType(before, afterPtr),
		Before:     cloneSnapshotPtr(before),
		After:      cloneSnapshotPtr(afterPtr),
	}
	return r.store.SaveCheckpoint(ctx, domainhistory.Checkpoint{
		Workspace: r.workspace,
		SessionID: command.SessionID,
		RequestID: command.RequestID,
		Title:     command.Title,
		Reason:    command.Reason,
	}, []domainhistory.FileChange{change})
}

func (r *Recorder) SnapshotPath(path string) (*domainhistory.FileSnapshot, error) {
	if r == nil {
		return nil, nil
	}

	rel, abs, err := r.cleanPath(path)
	if err != nil {
		return nil, err
	}
	snapshot, exists, err := SnapshotFile(abs, rel)
	if err != nil || !exists {
		return nil, err
	}
	return &snapshot, nil
}

func (r *Recorder) cleanPath(path string) (string, string, error) {
	if r == nil {
		return "", "", errors.New("history recorder is nil")
	}
	return cleanRecorderPath(r.workspace, path)
}
