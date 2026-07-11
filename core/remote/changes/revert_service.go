package changes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	domainhistory "myai/core/domain/history"
	"myai/core/remote/protocol"
)

func (s *Service) Revert(ctx context.Context, payload protocol.ChangeRevertPayload) (protocol.ChangeRevertResultPayload, error) {
	rel, abs, err := cleanPath(s.root, payload.Path)
	if err != nil {
		return protocol.ChangeRevertResultPayload{}, err
	}
	if shouldHidePath(rel) {
		return protocol.ChangeRevertResultPayload{}, fmt.Errorf("refusing to revert sensitive change: %s", rel)
	}
	if err := ctx.Err(); err != nil {
		return protocol.ChangeRevertResultPayload{}, err
	}

	s.mu.RLock()
	base, ok := s.baseline[rel]
	s.mu.RUnlock()
	// baseline 中不存在表示它是新文件，恢复动作应删除它；否则写回 baseline 内容。
	if !ok {
		info, err := os.Stat(abs)
		if errors.Is(err, os.ErrNotExist) {
			return protocol.ChangeRevertResultPayload{}, fmt.Errorf("new file no longer exists: %s", rel)
		}
		if err != nil {
			return protocol.ChangeRevertResultPayload{}, err
		}
		if info.IsDir() {
			return protocol.ChangeRevertResultPayload{}, fmt.Errorf("cannot revert directory: %s", rel)
		}
		if err := os.Remove(abs); err != nil {
			return protocol.ChangeRevertResultPayload{}, err
		}
		return protocol.ChangeRevertResultPayload{
			Path:     rel,
			Reverted: true,
			Message:  "New file removed from the workspace.",
		}, nil
	}
	if !base.Available {
		return protocol.ChangeRevertResultPayload{}, fmt.Errorf("baseline content is not available for: %s", rel)
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return protocol.ChangeRevertResultPayload{}, err
	}
	if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(abs); err != nil {
			return protocol.ChangeRevertResultPayload{}, err
		}
	}
	if err := os.WriteFile(abs, base.Content, base.Mode.Perm()); err != nil {
		return protocol.ChangeRevertResultPayload{}, err
	}

	return protocol.ChangeRevertResultPayload{
		Path:     rel,
		Reverted: true,
		Message:  "File restored to the SQLite workspace history baseline.",
	}, nil
}

func (s *Service) RevertCheckpoint(ctx context.Context, payload protocol.HistoryRevertPayload) (protocol.HistoryRevertResultPayload, error) {
	changes, err := s.store.LoadCheckpointChanges(ctx, s.root, payload.CheckpointID)
	if err != nil {
		return protocol.HistoryRevertResultPayload{}, err
	}
	if len(changes) == 0 {
		return protocol.HistoryRevertResultPayload{}, fmt.Errorf("checkpoint has no file changes: %s", payload.CheckpointID)
	}

	reverted := make([]string, 0, len(changes))
	// 检查点按逆序恢复，正确撤销同一任务内可能存在顺序依赖的文件操作。
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		if shouldHidePath(change.Path) {
			return protocol.HistoryRevertResultPayload{}, fmt.Errorf("refusing to revert sensitive change: %s", change.Path)
		}
		if err := s.revertStoredChange(ctx, change); err != nil {
			return protocol.HistoryRevertResultPayload{}, err
		}
		reverted = append(reverted, change.Path)
	}

	return protocol.HistoryRevertResultPayload{
		CheckpointID: payload.CheckpointID,
		Reverted:     true,
		Paths:        reverted,
		Message:      fmt.Sprintf("Reverted checkpoint %s.", payload.CheckpointID),
	}, nil
}

func (s *Service) revertStoredChange(ctx context.Context, change domainhistory.StoredFileChange) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rel, abs, err := cleanPath(s.root, change.Path)
	if err != nil {
		return err
	}
	if change.Before == nil {
		info, err := os.Stat(abs)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("cannot revert directory: %s", rel)
		}
		return os.Remove(abs)
	}
	if !change.Before.Available {
		return fmt.Errorf("checkpoint content is not available for: %s", rel)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(abs); err != nil {
			return err
		}
	}
	return os.WriteFile(abs, change.Before.Content, change.Before.Mode.Perm())
}
