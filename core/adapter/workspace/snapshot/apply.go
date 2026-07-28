package snapshot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	domainhistory "myai/core/domain/history"
	domainworkspace "myai/core/domain/workspace"
	"myai/core/history"
	workspaceport "myai/core/port/workspace"
)

type rollbackEntry struct {
	Path    string
	Existed bool
	Backup  string
	Mode    os.FileMode
}

func (manager *Manager) conflicts(value manifest, changes []domainworkspace.FileChange) ([]string, error) {
	baseline := stateMap(value.BaselineFiles)
	conflicts := make([]string, 0)
	for _, change := range changes {
		current, exists, err := inspectWorkspaceFile(value.SourceRoot, change.Path)
		if err != nil {
			return nil, err
		}
		before, hadBefore := baseline[change.Path]
		switch {
		case !hadBefore && exists:
			conflicts = append(conflicts, change.Path)
		case hadBefore && (!exists || before.Hash != current.Hash || before.Size != current.Size):
			conflicts = append(conflicts, change.Path)
		}
	}
	sort.Strings(conflicts)
	return conflicts, nil
}

func (manager *Manager) applyFiles(ctx context.Context, value manifest, changes []domainworkspace.FileChange, request workspaceport.ApplyRequest) (string, []rollbackEntry, error) {
	if len(changes) == 0 {
		return "", nil, nil
	}
	jobRoot, err := manager.jobRoot(value.WorkspaceID)
	if err != nil {
		return "", nil, err
	}
	rollbackRoot := filepath.Join(jobRoot, "rollback")
	if err := os.RemoveAll(rollbackRoot); err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(rollbackRoot, 0o700); err != nil {
		return "", nil, err
	}

	entries := make([]rollbackEntry, 0, len(changes))
	historyChanges := make([]domainhistory.FileChange, 0, len(changes))
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			_ = rollback(value.SourceRoot, entries)
			return "", nil, err
		}
		destination, err := safeJoin(value.SourceRoot, change.Path)
		if err != nil {
			_ = rollback(value.SourceRoot, entries)
			return "", nil, err
		}
		before, beforeExists, err := history.SnapshotFile(destination, change.Path)
		if err != nil {
			_ = rollback(value.SourceRoot, entries)
			return "", nil, err
		}
		entry, err := backupFile(ctx, destination, filepath.Join(rollbackRoot, filepath.FromSlash(change.Path)))
		if err != nil {
			_ = rollback(value.SourceRoot, entries)
			return "", nil, err
		}
		entry.Path = change.Path
		entries = append(entries, entry)

		var after *domainhistory.FileSnapshot
		if change.ChangeType == domainworkspace.FileChangeDeleted {
			if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
				_ = rollback(value.SourceRoot, entries)
				return "", nil, err
			}
		} else {
			source, err := safeJoin(value.SnapshotRoot, change.Path)
			if err != nil {
				_ = rollback(value.SourceRoot, entries)
				return "", nil, err
			}
			if err := copyAtomically(ctx, source, destination); err != nil {
				_ = rollback(value.SourceRoot, entries)
				return "", nil, err
			}
			snapshot, exists, err := history.SnapshotFile(source, change.Path)
			if err != nil || !exists {
				_ = rollback(value.SourceRoot, entries)
				if err != nil {
					return "", nil, err
				}
				return "", nil, fmt.Errorf("snapshot result file disappeared: %s", change.Path)
			}
			after = &snapshot
		}
		var beforePtr *domainhistory.FileSnapshot
		if beforeExists {
			beforePtr = &before
		}
		historyChanges = append(historyChanges, domainhistory.FileChange{
			Path: change.Path, ChangeType: string(change.ChangeType), Before: beforePtr, After: after,
		})
	}

	checkpointID, err := manager.saveCheckpoint(ctx, value.SourceRoot, historyChanges, request)
	if err != nil {
		rollbackErr := rollback(value.SourceRoot, entries)
		if rollbackErr != nil {
			return "", nil, errors.Join(err, rollbackErr)
		}
		return "", nil, err
	}
	return checkpointID, entries, nil
}

func backupFile(ctx context.Context, source string, destination string) (rollbackEntry, error) {
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return rollbackEntry{}, nil
	}
	if err != nil {
		return rollbackEntry{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return rollbackEntry{}, fmt.Errorf("workspace path is not a regular file: %s", source)
	}
	if _, err := copyFileWithHash(ctx, source, destination, info.Mode().Perm()); err != nil {
		return rollbackEntry{}, err
	}
	return rollbackEntry{Existed: true, Backup: destination, Mode: info.Mode().Perm()}, nil
}

func copyAtomically(ctx context.Context, source string, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("snapshot result is not a regular file: %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".myai-subagent-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	defer os.Remove(temporaryPath)
	if err := os.Remove(temporaryPath); err != nil {
		return err
	}
	if _, err := copyFileWithHash(ctx, source, temporaryPath, info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryPath, destination)
}

func rollback(sourceRoot string, entries []rollbackEntry) error {
	var errs []error
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		destination, err := safeJoin(sourceRoot, entry.Path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if entry.Existed {
			if err := copyAtomically(context.Background(), entry.Backup, destination); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (manager *Manager) saveCheckpoint(ctx context.Context, sourceRoot string, changes []domainhistory.FileChange, request workspaceport.ApplyRequest) (string, error) {
	path, err := manager.history.DefaultPath(sourceRoot)
	if err != nil {
		return "", err
	}
	store, err := manager.history.Open(path)
	if err != nil {
		return "", err
	}
	defer store.Close()
	return store.SaveCheckpoint(ctx, domainhistory.Checkpoint{
		Workspace: sourceRoot, SessionID: request.SessionID, RequestID: request.RequestID,
		Title: request.Title, Reason: "subagent snapshot changes applied", CreatedAt: manager.currentTime(),
	}, changes)
}

func (manager *Manager) deleteCheckpoint(ctx context.Context, sourceRoot string, checkpointID string) error {
	path, err := manager.history.DefaultPath(sourceRoot)
	if err != nil {
		return err
	}
	store, err := manager.history.Open(path)
	if err != nil {
		return err
	}
	defer store.Close()
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return store.DeleteCheckpoint(cleanupContext, sourceRoot, checkpointID)
}
