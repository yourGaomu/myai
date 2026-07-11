package changes

import (
	"context"
	"fmt"
	"path/filepath"

	domainhistory "myai/core/domain/history"
	"myai/core/remote/protocol"
)

func (s *Service) History(ctx context.Context, payload protocol.HistoryListPayload) (protocol.HistoryListResultPayload, error) {
	limit := payload.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	checkpoints, err := s.store.ListCheckpoints(ctx, s.root, limit)
	if err != nil {
		return protocol.HistoryListResultPayload{}, err
	}

	items := make([]protocol.HistoryCheckpoint, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		items = append(items, protocol.HistoryCheckpoint{
			ID:          checkpoint.ID,
			Title:       checkpoint.Title,
			Reason:      checkpoint.Reason,
			SessionID:   checkpoint.SessionID,
			RequestID:   checkpoint.RequestID,
			ChangeCount: checkpoint.ChangeCount,
			CreatedAt:   checkpoint.CreatedAt,
		})
	}

	return protocol.HistoryListResultPayload{
		Root:        filepath.ToSlash(s.root),
		Checkpoints: items,
		Count:       len(items),
	}, nil
}

func (s *Service) HistoryDiff(ctx context.Context, payload protocol.HistoryDiffPayload) (protocol.HistoryDiffResultPayload, error) {
	// 历史 Diff 来自检查点保存的 before/after，不依赖当前文件是否仍然存在。
	changes, err := s.store.LoadCheckpointChanges(ctx, s.root, payload.CheckpointID)
	if err != nil {
		return protocol.HistoryDiffResultPayload{}, err
	}
	if len(changes) == 0 {
		return protocol.HistoryDiffResultPayload{}, fmt.Errorf("checkpoint has no file changes: %s", payload.CheckpointID)
	}

	files := make([]protocol.HistoryFileDiff, 0, len(changes))
	for _, change := range changes {
		if shouldHidePath(change.Path) {
			return protocol.HistoryDiffResultPayload{}, fmt.Errorf("refusing to preview sensitive change: %s", change.Path)
		}
		files = append(files, storedChangeDiff(change))
	}

	return protocol.HistoryDiffResultPayload{
		CheckpointID: payload.CheckpointID,
		Files:        files,
		Count:        len(files),
	}, nil
}

func storedChangeDiff(change domainhistory.StoredFileChange) protocol.HistoryFileDiff {
	item := protocol.HistoryFileDiff{
		Path:       change.Path,
		ChangeType: change.ChangeType,
		Restorable: change.Before == nil || change.Before.Available,
	}

	switch {
	case change.Before == nil && change.After == nil:
		item.Message = "No diff is available for this file change."
	case change.Before == nil:
		diff, truncated, binary := historyAdditionDiff(change.After)
		item.Diff = diff
		item.Truncated = truncated
		item.Binary = binary
		item.Message = emptyDiffMessage(diff, binary)
	case change.After == nil:
		diff, truncated, binary := historyDeletionDiff(change.Before)
		item.Diff = diff
		item.Truncated = truncated
		item.Binary = binary
		item.Message = emptyDiffMessage(diff, binary)
	default:
		diff, truncated, binary := historyModifiedDiff(change.Before, change.After)
		item.Diff = diff
		item.Truncated = truncated
		item.Binary = binary
		item.Message = emptyDiffMessage(diff, binary)
	}
	return item
}
