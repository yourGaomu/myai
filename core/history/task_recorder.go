package history

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	domainhistory "myai/core/domain/history"
	historyport "myai/core/port/history"
)

type taskRecorderContextKey struct{}

type TaskRecorder struct {
	// TaskRecorder 聚合一次模型生成期间的多次文件操作，最终合并为一个用户可恢复检查点。
	mu         sync.Mutex
	command    RecordCommand
	store      historyport.Store
	factory    historyport.StoreFactory
	workspaces map[string]*taskWorkspaceState
	closed     bool
	saved      bool
}

type taskWorkspaceState struct {
	workspace string
	store     historyport.Store
	ownsStore bool
	changes   map[string]domainhistory.FileChange
	order     []string
}

type TaskWorkspaceRecorder struct {
	task  *TaskRecorder
	state *taskWorkspaceState
}

func NewTaskRecorder(command RecordCommand, factory historyport.StoreFactory) *TaskRecorder {
	return &TaskRecorder{
		command:    command,
		factory:    factory,
		workspaces: make(map[string]*taskWorkspaceState),
	}
}

func NewTaskRecorderWithStore(command RecordCommand, store historyport.Store) *TaskRecorder {
	return &TaskRecorder{
		command:    command,
		store:      store,
		workspaces: make(map[string]*taskWorkspaceState),
	}
}

func WithTaskRecorder(ctx context.Context, recorder *TaskRecorder) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, taskRecorderContextKey{}, recorder)
}

func TaskRecorderFromContext(ctx context.Context) *TaskRecorder {
	if ctx == nil {
		return nil
	}
	recorder, _ := ctx.Value(taskRecorderContextKey{}).(*TaskRecorder)
	return recorder
}

func (t *TaskRecorder) WorkspaceRecorder(workspace string) (*TaskWorkspaceRecorder, error) {
	if t == nil {
		return nil, errors.New("task recorder is nil")
	}

	abs, err := cleanWorkspace(workspace)
	if err != nil {
		return nil, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil, errors.New("task recorder is closed")
	}
	if t.workspaces == nil {
		t.workspaces = make(map[string]*taskWorkspaceState)
	}
	if state := t.workspaces[abs]; state != nil {
		return &TaskWorkspaceRecorder{task: t, state: state}, nil
	}

	store := t.store
	ownsStore := false
	if store == nil {
		if t.factory == nil {
			return nil, errors.New("history store factory is nil")
		}
		dbPath, err := t.factory.DefaultPath(abs)
		if err != nil {
			return nil, err
		}
		store, err = t.factory.Open(dbPath)
		if err != nil {
			return nil, err
		}
		ownsStore = true
	}

	state := &taskWorkspaceState{
		workspace: abs,
		store:     store,
		ownsStore: ownsStore,
		changes:   make(map[string]domainhistory.FileChange),
	}
	t.workspaces[abs] = state
	return &TaskWorkspaceRecorder{task: t, state: state}, nil
}

func (t *TaskRecorder) Save(ctx context.Context) ([]string, error) {
	if t == nil {
		return nil, nil
	}

	t.mu.Lock()
	// Save 幂等，defer 或异常收尾重复调用时不会生成重复检查点。
	if t.saved {
		t.mu.Unlock()
		return nil, nil
	}
	t.saved = true
	command := t.command
	workspaces := make([]*taskWorkspaceState, 0, len(t.workspaces))
	for _, state := range t.workspaces {
		workspaces = append(workspaces, state)
	}
	t.mu.Unlock()

	ids := make([]string, 0, len(workspaces))
	for _, state := range workspaces {
		changes := orderedTaskChanges(state)
		if len(changes) == 0 {
			continue
		}

		title := strings.TrimSpace(command.Title)
		if title == "" {
			title = taskCheckpointTitle(changes)
		}
		reason := strings.TrimSpace(command.Reason)
		if reason == "" {
			reason = "user request"
		}

		id, err := state.store.SaveCheckpoint(ctx, domainhistory.Checkpoint{
			Workspace: state.workspace,
			SessionID: command.SessionID,
			RequestID: command.RequestID,
			Title:     title,
			Reason:    reason,
		}, changes)
		if err != nil {
			return ids, err
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (t *TaskRecorder) Close() error {
	if t == nil {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true

	var closeErr error
	for _, state := range t.workspaces {
		if !state.ownsStore || state.store == nil {
			continue
		}
		if err := state.store.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (r *TaskWorkspaceRecorder) SnapshotPath(path string) (*domainhistory.FileSnapshot, error) {
	if r == nil || r.state == nil {
		return nil, nil
	}

	rel, abs, err := cleanRecorderPath(r.state.workspace, path)
	if err != nil {
		return nil, err
	}
	snapshot, exists, err := SnapshotFile(abs, rel)
	if err != nil || !exists {
		return nil, err
	}
	return &snapshot, nil
}

func (r *TaskWorkspaceRecorder) RecordFileChange(ctx context.Context, path string, before *domainhistory.FileSnapshot, _ RecordCommand) (string, error) {
	if r == nil || r.task == nil || r.state == nil {
		return "", nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	rel, abs, err := cleanRecorderPath(r.state.workspace, path)
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
	if !snapshotChanged(before, afterPtr) {
		return "", nil
	}

	r.task.addChange(r.state, domainhistory.FileChange{
		Path:       rel,
		ChangeType: changeType(before, afterPtr),
		Before:     cloneSnapshotPtr(before),
		After:      cloneSnapshotPtr(afterPtr),
	})
	return "", nil
}

func (r *TaskWorkspaceRecorder) SnapshotWorkspace(ctx context.Context) (map[string]domainhistory.FileSnapshot, error) {
	if r == nil || r.state == nil {
		return nil, nil
	}
	return snapshotWorkspace(ctx, r.state.workspace)
}

func (r *TaskWorkspaceRecorder) RecordWorkspaceChanges(ctx context.Context, before map[string]domainhistory.FileSnapshot, _ RecordCommand) (int, error) {
	if r == nil || r.task == nil || r.state == nil {
		return 0, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	after, err := r.SnapshotWorkspace(ctx)
	if err != nil {
		return 0, err
	}

	changed := 0
	for path, beforeSnapshot := range before {
		if _, ok := after[path]; ok {
			continue
		}
		r.task.addChange(r.state, domainhistory.FileChange{
			Path:       path,
			ChangeType: "deleted",
			Before:     cloneSnapshotPtr(&beforeSnapshot),
		})
		changed++
	}
	for path, afterSnapshot := range after {
		beforeSnapshot, ok := before[path]
		if ok && !snapshotChanged(&beforeSnapshot, &afterSnapshot) {
			continue
		}

		var beforePtr *domainhistory.FileSnapshot
		if ok {
			beforePtr = &beforeSnapshot
		}
		r.task.addChange(r.state, domainhistory.FileChange{
			Path:       path,
			ChangeType: changeType(beforePtr, &afterSnapshot),
			Before:     cloneSnapshotPtr(beforePtr),
			After:      cloneSnapshotPtr(&afterSnapshot),
		})
		changed++
	}
	return changed, nil
}

func (t *TaskRecorder) addChange(state *taskWorkspaceState, change domainhistory.FileChange) {
	if t == nil || state == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if state.changes == nil {
		state.changes = make(map[string]domainhistory.FileChange)
	}

	// 同一任务多次修改同一文件时保留最初 before 和最终 after，中间状态不进入历史。
	existing, ok := state.changes[change.Path]
	if !ok {
		state.order = append(state.order, change.Path)
		state.changes[change.Path] = cloneFileChange(change)
		return
	}

	merged := domainhistory.FileChange{
		Path:      change.Path,
		Before:    cloneSnapshotPtr(existing.Before),
		After:     cloneSnapshotPtr(change.After),
		CreatedAt: existing.CreatedAt,
	}
	merged.ChangeType = changeType(merged.Before, merged.After)
	if !snapshotChanged(merged.Before, merged.After) {
		delete(state.changes, change.Path)
		return
	}
	state.changes[change.Path] = merged
}

func orderedTaskChanges(state *taskWorkspaceState) []domainhistory.FileChange {
	if state == nil {
		return nil
	}
	changes := make([]domainhistory.FileChange, 0, len(state.changes))
	seen := make(map[string]bool, len(state.changes))
	for _, path := range state.order {
		if seen[path] {
			continue
		}
		seen[path] = true
		change, ok := state.changes[path]
		if !ok || !snapshotChanged(change.Before, change.After) {
			continue
		}
		change.ChangeType = changeType(change.Before, change.After)
		changes = append(changes, cloneFileChange(change))
	}
	return changes
}

func taskCheckpointTitle(changes []domainhistory.FileChange) string {
	if len(changes) == 0 {
		return "file changes"
	}
	if len(changes) == 1 {
		return "file change " + changes[0].Path
	}
	return fmt.Sprintf("file changes (%d files)", len(changes))
}
