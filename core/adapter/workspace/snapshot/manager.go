package snapshot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	domainworkspace "myai/core/domain/workspace"
	historyport "myai/core/port/history"
	workspaceport "myai/core/port/workspace"
)

var validWorkspaceID = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type Manager struct {
	root            string
	history         historyport.StoreFactory
	now             func() time.Time
	persistManifest func(string, manifest) error
	mu              sync.Mutex
}

var _ workspaceport.Manager = (*Manager)(nil)

func New(root string, history historyport.StoreFactory) (*Manager, error) {
	if history == nil {
		return nil, errors.New("snapshot workspace history factory is nil")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(workingDirectory, ".myai", "subagent-snapshots")
	}
	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	return &Manager{root: abs, history: history, persistManifest: saveManifest}, nil
}

func (manager *Manager) Prepare(ctx context.Context, request workspaceport.PrepareRequest) (workspaceport.PreparedWorkspace, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if request.Mode != domainworkspace.IsolationModeSnapshot {
		return workspaceport.PreparedWorkspace{}, fmt.Errorf("snapshot manager does not support isolation mode %q", request.Mode)
	}
	jobRoot, err := manager.jobRoot(request.WorkspaceID)
	if err != nil {
		return workspaceport.PreparedWorkspace{}, err
	}
	if stored, err := loadManifest(jobRoot); err == nil {
		return manager.reopenExisting(ctx, jobRoot, stored)
	} else if !errors.Is(err, os.ErrNotExist) {
		return workspaceport.PreparedWorkspace{}, err
	}
	source, err := normalizeDirectory(request.SourceRoot)
	if err != nil {
		return workspaceport.PreparedWorkspace{}, err
	}
	if err := os.Mkdir(jobRoot, 0o700); err != nil {
		return workspaceport.PreparedWorkspace{}, err
	}
	snapshotRoot := filepath.Join(jobRoot, "workspace")
	if err := os.Mkdir(snapshotRoot, 0o700); err != nil {
		_ = manager.removeJobRoot(jobRoot)
		return workspaceport.PreparedWorkspace{}, err
	}
	baseline, err := copyWorkspace(ctx, source, snapshotRoot, manager.root)
	if err != nil {
		_ = manager.removeJobRoot(jobRoot)
		return workspaceport.PreparedWorkspace{}, fmt.Errorf("prepare snapshot workspace: %w", err)
	}
	value := manifest{
		Version: 1, WorkspaceID: request.WorkspaceID, SourceRoot: source, SnapshotRoot: snapshotRoot,
		Status: domainworkspace.ChangeSetStatusPending, CreatedAt: manager.currentTime(), BaselineFiles: baseline,
	}
	if err := manager.saveManifest(jobRoot, value); err != nil {
		_ = manager.removeJobRoot(jobRoot)
		return workspaceport.PreparedWorkspace{}, err
	}
	return workspaceport.PreparedWorkspace{Reference: snapshotReference(value)}, nil
}

func (manager *Manager) reopenExisting(ctx context.Context, jobRoot string, stored manifest) (workspaceport.PreparedWorkspace, error) {
	if stored.Status == domainworkspace.ChangeSetStatusApplied {
		scanned, err := scanWorkspace(ctx, stored.SnapshotRoot)
		if err != nil {
			return workspaceport.PreparedWorkspace{}, err
		}
		baseline := make([]fileState, 0, len(scanned))
		for _, file := range scanned {
			baseline = append(baseline, file)
		}
		sort.Slice(baseline, func(left, right int) bool { return baseline[left].Path < baseline[right].Path })
		stored.BaselineFiles = baseline
		stored.Status = domainworkspace.ChangeSetStatusPending
		stored.AppliedAt = nil
		stored.CheckpointID = ""
		stored.CreatedAt = manager.currentTime()
		if err := manager.saveManifest(jobRoot, stored); err != nil {
			return workspaceport.PreparedWorkspace{}, err
		}
	}
	return workspaceport.PreparedWorkspace{Reference: snapshotReference(stored)}, nil
}

func snapshotReference(stored manifest) domainworkspace.Reference {
	return domainworkspace.Reference{
		ID: stored.WorkspaceID, Mode: domainworkspace.IsolationModeSnapshot,
		Root: stored.SnapshotRoot, SourceRoot: stored.SourceRoot,
	}
}

func (manager *Manager) Collect(ctx context.Context, request workspaceport.CollectRequest) (workspaceport.CollectedChanges, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	value, err := manager.load(request.Reference)
	if err != nil {
		return workspaceport.CollectedChanges{}, err
	}
	changeSet, err := manager.collect(ctx, value)
	return workspaceport.CollectedChanges{ChangeSet: changeSet}, err
}

func (manager *Manager) collect(ctx context.Context, value manifest) (domainworkspace.ChangeSet, error) {
	current, err := scanWorkspace(ctx, value.SnapshotRoot)
	if err != nil {
		return domainworkspace.ChangeSet{}, err
	}
	baseline := stateMap(value.BaselineFiles)
	paths := make(map[string]struct{}, len(baseline)+len(current))
	for path := range baseline {
		paths[path] = struct{}{}
	}
	for path := range current {
		paths[path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	files := make([]domainworkspace.FileChange, 0)
	for _, path := range ordered {
		before, hadBefore := baseline[path]
		after, hasAfter := current[path]
		if hadBefore && hasAfter && before.Hash == after.Hash && before.Size == after.Size {
			continue
		}
		change := domainworkspace.FileChange{Path: path}
		if hadBefore {
			change.BeforeHash = before.Hash
			change.BeforeSize = before.Size
		}
		if hasAfter {
			change.AfterHash = after.Hash
			change.AfterSize = after.Size
		}
		switch {
		case !hadBefore:
			change.ChangeType = domainworkspace.FileChangeAdded
		case !hasAfter:
			change.ChangeType = domainworkspace.FileChangeDeleted
		default:
			change.ChangeType = domainworkspace.FileChangeModified
		}
		files = append(files, change)
	}
	status := value.Status
	if status == "" {
		status = domainworkspace.ChangeSetStatusPending
	}
	return domainworkspace.ChangeSet{
		WorkspaceID: value.WorkspaceID, Status: status, Files: files,
		CheckpointID: value.CheckpointID, CreatedAt: value.CreatedAt, AppliedAt: value.AppliedAt,
	}, nil
}

func (manager *Manager) Apply(ctx context.Context, request workspaceport.ApplyRequest) (workspaceport.AppliedChanges, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	value, err := manager.load(request.Reference)
	if err != nil {
		return workspaceport.AppliedChanges{}, err
	}
	changeSet, err := manager.collect(ctx, value)
	if err != nil {
		return workspaceport.AppliedChanges{}, err
	}
	if value.Status == domainworkspace.ChangeSetStatusApplied {
		return workspaceport.AppliedChanges{ChangeSet: changeSet}, nil
	}
	if value.Status == domainworkspace.ChangeSetStatusDiscarded {
		return workspaceport.AppliedChanges{}, errors.New("snapshot workspace was discarded")
	}
	conflicts, err := manager.conflicts(value, changeSet.Files)
	if err != nil {
		return workspaceport.AppliedChanges{}, err
	}
	if len(conflicts) > 0 {
		changeSet.Status = domainworkspace.ChangeSetStatusConflict
		changeSet.Message = "source workspace changed after the subagent snapshot: " + strings.Join(conflicts, ", ")
		return workspaceport.AppliedChanges{ChangeSet: changeSet}, errors.Join(workspaceport.ErrConflict, errors.New(changeSet.Message))
	}
	checkpointID, rollbackEntries, err := manager.applyFiles(ctx, value, changeSet.Files, request)
	if err != nil {
		return workspaceport.AppliedChanges{}, err
	}
	now := manager.currentTime()
	value.Status = domainworkspace.ChangeSetStatusApplied
	value.AppliedAt = &now
	value.CheckpointID = checkpointID
	jobRoot, _ := manager.jobRoot(value.WorkspaceID)
	if err := manager.saveManifest(jobRoot, value); err != nil {
		rollbackErr := rollback(value.SourceRoot, rollbackEntries)
		if rollbackErr != nil {
			return workspaceport.AppliedChanges{}, errors.Join(err, fmt.Errorf("rollback snapshot changes: %w", rollbackErr))
		}
		if checkpointID != "" {
			if deleteErr := manager.deleteCheckpoint(ctx, value.SourceRoot, checkpointID); deleteErr != nil {
				return workspaceport.AppliedChanges{}, errors.Join(err, fmt.Errorf("delete rolled back checkpoint: %w", deleteErr))
			}
		}
		return workspaceport.AppliedChanges{}, err
	}
	changeSet.Status = domainworkspace.ChangeSetStatusApplied
	changeSet.AppliedAt = &now
	changeSet.CheckpointID = checkpointID
	return workspaceport.AppliedChanges{ChangeSet: changeSet}, nil
}

func (manager *Manager) Discard(ctx context.Context, request workspaceport.DiscardRequest) (workspaceport.DiscardedChanges, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	value, err := manager.load(request.Reference)
	if err != nil {
		return workspaceport.DiscardedChanges{}, err
	}
	changeSet, err := manager.collect(ctx, value)
	if err != nil {
		return workspaceport.DiscardedChanges{}, err
	}
	if value.Status == domainworkspace.ChangeSetStatusApplied {
		return workspaceport.DiscardedChanges{}, errors.New("applied snapshot workspace cannot be discarded")
	}
	discardedAt := request.DiscardedAt
	if discardedAt.IsZero() {
		discardedAt = manager.currentTime()
	}
	jobRoot, _ := manager.jobRoot(value.WorkspaceID)
	if err := manager.removeJobRoot(jobRoot); err != nil {
		return workspaceport.DiscardedChanges{}, err
	}
	changeSet.Status = domainworkspace.ChangeSetStatusDiscarded
	changeSet.DiscardedAt = &discardedAt
	return workspaceport.DiscardedChanges{ChangeSet: changeSet}, nil
}

func (manager *Manager) load(reference domainworkspace.Reference) (manifest, error) {
	if reference.Mode != domainworkspace.IsolationModeSnapshot {
		return manifest{}, fmt.Errorf("snapshot manager cannot load isolation mode %q", reference.Mode)
	}
	jobRoot, err := manager.jobRoot(reference.ID)
	if err != nil {
		return manifest{}, err
	}
	value, err := loadManifest(jobRoot)
	if err != nil {
		return manifest{}, err
	}
	if filepath.Clean(value.SnapshotRoot) != filepath.Clean(reference.Root) {
		return manifest{}, errors.New("snapshot workspace reference does not match its manifest")
	}
	return value, nil
}

func (manager *Manager) jobRoot(id string) (string, error) {
	id = strings.TrimSpace(id)
	if !validWorkspaceID.MatchString(id) || id == "." || id == ".." {
		return "", fmt.Errorf("invalid snapshot workspace id %q", id)
	}
	target := filepath.Join(manager.root, id)
	relative, err := filepath.Rel(manager.root, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("snapshot workspace path escapes storage root")
	}
	return target, nil
}

func (manager *Manager) removeJobRoot(path string) error {
	relative, err := filepath.Rel(manager.root, filepath.Clean(path))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("refusing to remove snapshot storage root or an external path")
	}
	return os.RemoveAll(path)
}

func (manager *Manager) currentTime() time.Time {
	if manager.now != nil {
		return manager.now().UTC()
	}
	return time.Now().UTC()
}

func (manager *Manager) saveManifest(path string, value manifest) error {
	if manager.persistManifest != nil {
		return manager.persistManifest(path, value)
	}
	return saveManifest(path, value)
}
