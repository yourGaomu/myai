package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	maxRecordedFileBytes = 512 * 1024
)

var ignoredSnapshotNames = map[string]bool{
	".git":         true,
	".idea":        true,
	".expo":        true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".cache":       true,
}

var sensitiveSnapshotNames = map[string]bool{
	".env":            true,
	".env.local":      true,
	".env.production": true,
	"id_rsa":          true,
	"id_ed25519":      true,
}

type Recorder struct {
	workspace string
	store     *SQLiteStore
}

type RecordOptions struct {
	Title     string
	Reason    string
	SessionID string
	RequestID string
}

type taskRecorderContextKey struct{}

type TaskRecorder struct {
	mu         sync.Mutex
	options    RecordOptions
	store      *SQLiteStore
	workspaces map[string]*taskWorkspaceState
	closed     bool
	saved      bool
}

type taskWorkspaceState struct {
	workspace string
	store     *SQLiteStore
	ownsStore bool
	changes   map[string]FileChange
	order     []string
}

type TaskWorkspaceRecorder struct {
	task  *TaskRecorder
	state *taskWorkspaceState
}

func NewRecorder(workspace string) (*Recorder, error) {
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
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory: %s", abs)
	}

	dbPath, err := DefaultSQLitePath(abs)
	if err != nil {
		return nil, err
	}
	store, err := OpenSQLite(dbPath)
	if err != nil {
		return nil, err
	}

	return &Recorder{
		workspace: abs,
		store:     store,
	}, nil
}

func NewRecorderWithStore(workspace string, store *SQLiteStore) (*Recorder, error) {
	if store == nil {
		return nil, errors.New("sqlite history store is nil")
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

func NewTaskRecorder(options RecordOptions) *TaskRecorder {
	return &TaskRecorder{
		options:    options,
		workspaces: make(map[string]*taskWorkspaceState),
	}
}

func NewTaskRecorderWithStore(options RecordOptions, store *SQLiteStore) *TaskRecorder {
	return &TaskRecorder{
		options:    options,
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
		dbPath, err := DefaultSQLitePath(abs)
		if err != nil {
			return nil, err
		}
		store, err = OpenSQLite(dbPath)
		if err != nil {
			return nil, err
		}
		ownsStore = true
	}

	state := &taskWorkspaceState{
		workspace: abs,
		store:     store,
		ownsStore: ownsStore,
		changes:   make(map[string]FileChange),
	}
	t.workspaces[abs] = state
	return &TaskWorkspaceRecorder{task: t, state: state}, nil
}

func (t *TaskRecorder) Save(ctx context.Context) ([]string, error) {
	if t == nil {
		return nil, nil
	}

	t.mu.Lock()
	if t.saved {
		t.mu.Unlock()
		return nil, nil
	}
	t.saved = true
	options := t.options
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

		title := strings.TrimSpace(options.Title)
		if title == "" {
			title = taskCheckpointTitle(changes)
		}
		reason := strings.TrimSpace(options.Reason)
		if reason == "" {
			reason = "user request"
		}

		id, err := state.store.SaveCheckpoint(ctx, Checkpoint{
			Workspace: state.workspace,
			SessionID: options.SessionID,
			RequestID: options.RequestID,
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

func (r *Recorder) Close() error {
	if r == nil || r.store == nil {
		return nil
	}
	return r.store.Close()
}

func (r *Recorder) RecordFileChange(ctx context.Context, path string, before *FileSnapshot, options RecordOptions) (string, error) {
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

	var afterPtr *FileSnapshot
	if exists {
		afterPtr = &after
	}
	if !snapshotChanged(before, afterPtr) {
		return "", nil
	}

	change := FileChange{
		Path:       rel,
		ChangeType: changeType(before, afterPtr),
		Before:     cloneSnapshotPtr(before),
		After:      cloneSnapshotPtr(afterPtr),
	}
	return r.store.SaveCheckpoint(ctx, Checkpoint{
		Workspace: r.workspace,
		SessionID: options.SessionID,
		RequestID: options.RequestID,
		Title:     options.Title,
		Reason:    options.Reason,
	}, []FileChange{change})
}

func (r *Recorder) SnapshotPath(path string) (*FileSnapshot, error) {
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

func (r *TaskWorkspaceRecorder) SnapshotPath(path string) (*FileSnapshot, error) {
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

func (r *TaskWorkspaceRecorder) RecordFileChange(ctx context.Context, path string, before *FileSnapshot, options RecordOptions) (string, error) {
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

	var afterPtr *FileSnapshot
	if exists {
		afterPtr = &after
	}
	if !snapshotChanged(before, afterPtr) {
		return "", nil
	}

	r.task.addChange(r.state, FileChange{
		Path:       rel,
		ChangeType: changeType(before, afterPtr),
		Before:     cloneSnapshotPtr(before),
		After:      cloneSnapshotPtr(afterPtr),
	})
	return "", nil
}

func (r *TaskWorkspaceRecorder) SnapshotWorkspace(ctx context.Context) (map[string]FileSnapshot, error) {
	if r == nil || r.state == nil {
		return nil, nil
	}
	return snapshotWorkspace(ctx, r.state.workspace)
}

func (r *TaskWorkspaceRecorder) RecordWorkspaceChanges(ctx context.Context, before map[string]FileSnapshot, options RecordOptions) (int, error) {
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
		_, ok := after[path]
		if ok {
			continue
		}
		r.task.addChange(r.state, FileChange{
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

		var beforePtr *FileSnapshot
		if ok {
			beforePtr = &beforeSnapshot
		}
		r.task.addChange(r.state, FileChange{
			Path:       path,
			ChangeType: changeType(beforePtr, &afterSnapshot),
			Before:     cloneSnapshotPtr(beforePtr),
			After:      cloneSnapshotPtr(&afterSnapshot),
		})
		changed++
	}
	return changed, nil
}

func SnapshotFile(abs string, rel string) (FileSnapshot, bool, error) {
	info, err := os.Stat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return FileSnapshot{}, false, nil
	}
	if err != nil {
		return FileSnapshot{}, false, err
	}
	if info.IsDir() {
		return FileSnapshot{}, false, nil
	}

	file, err := os.Open(abs)
	if err != nil {
		return FileSnapshot{}, false, err
	}
	defer file.Close()

	content, digest, err := readSnapshotContent(file, info.Size())
	if err != nil {
		return FileSnapshot{}, false, err
	}

	snapshot := FileSnapshot{
		Path:      filepath.ToSlash(rel),
		Size:      info.Size(),
		Hash:      digest,
		Binary:    isBinary(content),
		TooLarge:  info.Size() > int64(maxRecordedFileBytes),
		Mode:      info.Mode(),
		Available: false,
	}
	snapshot.Available = !snapshot.Binary && !snapshot.TooLarge
	if snapshot.Available {
		snapshot.Content = append([]byte(nil), content...)
	}
	return snapshot, true, nil
}

func snapshotWorkspace(ctx context.Context, workspace string) (map[string]FileSnapshot, error) {
	result := make(map[string]FileSnapshot)
	err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == workspace {
			return nil
		}

		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if shouldSkipSnapshotPath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err != nil || !insideRoot(workspace, target) {
				return nil
			}
			path = target
		}

		snapshot, exists, err := SnapshotFile(path, rel)
		if err != nil {
			return err
		}
		if exists {
			result[rel] = snapshot
		}
		return nil
	})
	return result, err
}

func readSnapshotContent(file *os.File, size int64) ([]byte, [32]byte, error) {
	hasher := sha256.New()
	limit := maxRecordedFileBytes
	if size < int64(limit) {
		limit = int(size)
	}
	content := make([]byte, limit)
	n, err := io.ReadFull(io.TeeReader(file, hasher), content)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, [32]byte{}, err
	}
	content = content[:n]
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, [32]byte{}, err
	}
	return content, hashSum(hasher), nil
}

func hashSum(hasher hash.Hash) [32]byte {
	var digest [32]byte
	copy(digest[:], hasher.Sum(nil))
	return digest
}

func snapshotChanged(before *FileSnapshot, after *FileSnapshot) bool {
	if before == nil && after == nil {
		return false
	}
	if before == nil || after == nil {
		return true
	}
	return before.Size != after.Size ||
		before.Hash != after.Hash ||
		before.Binary != after.Binary ||
		before.TooLarge != after.TooLarge
}

func changeType(before *FileSnapshot, after *FileSnapshot) string {
	switch {
	case before == nil && after != nil:
		return "added"
	case before != nil && after == nil:
		return "deleted"
	default:
		return "modified"
	}
}

func cloneSnapshotPtr(source *FileSnapshot) *FileSnapshot {
	if source == nil {
		return nil
	}
	copied := *source
	if source.Content != nil {
		copied.Content = append([]byte(nil), source.Content...)
	}
	return &copied
}

func (t *TaskRecorder) addChange(state *taskWorkspaceState, change FileChange) {
	if t == nil || state == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if state.changes == nil {
		state.changes = make(map[string]FileChange)
	}

	existing, ok := state.changes[change.Path]
	if !ok {
		state.order = append(state.order, change.Path)
		state.changes[change.Path] = cloneFileChange(change)
		return
	}

	merged := FileChange{
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

func orderedTaskChanges(state *taskWorkspaceState) []FileChange {
	if state == nil {
		return nil
	}
	changes := make([]FileChange, 0, len(state.changes))
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

func cloneFileChange(source FileChange) FileChange {
	return FileChange{
		Path:       source.Path,
		ChangeType: source.ChangeType,
		Before:     cloneSnapshotPtr(source.Before),
		After:      cloneSnapshotPtr(source.After),
		CreatedAt:  source.CreatedAt,
	}
}

func taskCheckpointTitle(changes []FileChange) string {
	if len(changes) == 0 {
		return "file changes"
	}
	if len(changes) == 1 {
		return "file change " + changes[0].Path
	}
	return fmt.Sprintf("file changes (%d files)", len(changes))
}

func cleanWorkspace(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = "."
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", abs)
	}
	return abs, nil
}

func cleanRecorderPath(workspace string, path string) (string, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", errors.New("path is empty")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", "", err
	}
	if !insideRoot(workspace, abs) {
		return "", "", fmt.Errorf("path is outside workspace: %s", path)
	}
	rel, err := filepath.Rel(workspace, abs)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(rel), abs, nil
}

func shouldSkipSnapshotPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		lower := strings.ToLower(part)
		if ignoredSnapshotNames[lower] || sensitiveSnapshotNames[lower] {
			return true
		}
	}
	return false
}

func insideRoot(root string, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func isBinary(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return true
	}
	return !utf8.Valid(content)
}
