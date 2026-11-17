package changes

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"myai/core/remote/protocol"
)

const (
	defaultChangeLimit = 200
	maxChangeLimit     = 1000
)

func (s *Service) List(ctx context.Context, payload protocol.ChangesListPayload) (protocol.ChangesListResultPayload, error) {
	s.operation.Lock()
	defer s.operation.Unlock()
	limit := payload.Limit
	if limit <= 0 {
		limit = defaultChangeLimit
	}
	if limit > maxChangeLimit {
		limit = maxChangeLimit
	}

	// 当前快照与持久化 baseline 比较，得到 added/modified/deleted 三类变更。
	current, err := s.scan(ctx)
	if err != nil {
		return protocol.ChangesListResultPayload{}, err
	}

	s.mu.RLock()
	baseline := copySnapshot(s.baseline)
	s.mu.RUnlock()

	entries := make([]protocol.ChangeEntry, 0)
	for path, base := range baseline {
		if _, ok := current[path]; !ok {
			_, removed := entryLineCounts(&base, nil)
			entries = append(entries, protocol.ChangeEntry{
				Path:         path,
				Status:       "deleted",
				Deleted:      true,
				Unstaged:     true,
				Restorable:   base.Available,
				RemovedLines: removed,
			})
		}
	}
	for path, now := range current {
		base, ok := baseline[path]
		switch {
		case !ok:
			added, _ := entryLineCounts(nil, &now)
			entries = append(entries, protocol.ChangeEntry{
				Path:       path,
				Status:     "added",
				Untracked:  true,
				Unstaged:   true,
				Restorable: true,
				AddedLines: added,
			})
		case base.Hash != now.Hash || base.Size != now.Size || base.Binary != now.Binary || base.TooLarge != now.TooLarge:
			added, removed := entryLineCounts(&base, &now)
			entries = append(entries, protocol.ChangeEntry{
				Path:         path,
				Status:       "modified",
				Unstaged:     true,
				Restorable:   base.Available,
				AddedLines:   added,
				RemovedLines: removed,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Path) < strings.ToLower(entries[j].Path)
	})

	truncated := false
	if len(entries) > limit {
		entries = entries[:limit]
		truncated = true
	}

	return protocol.ChangesListResultPayload{
		Repository: false,
		Source:     "sqlite",
		Root:       filepath.ToSlash(s.root),
		Entries:    entries,
		Count:      len(entries),
		Truncated:  truncated,
		Clean:      len(entries) == 0,
		Message:    "Changes are compared with the SQLite workspace history baseline.",
	}, nil
}

func (s *Service) Diff(ctx context.Context, payload protocol.ChangeDiffPayload) (protocol.ChangeDiffResultPayload, error) {
	s.operation.Lock()
	defer s.operation.Unlock()
	if err := ctx.Err(); err != nil {
		return protocol.ChangeDiffResultPayload{}, err
	}
	rel, _, err := cleanPath(s.root, payload.Path)
	if err != nil {
		return protocol.ChangeDiffResultPayload{}, err
	}
	if shouldHidePath(rel) {
		return protocol.ChangeDiffResultPayload{}, fmt.Errorf("refusing to preview sensitive change: %s", rel)
	}

	s.mu.RLock()
	base, hadBase := s.baseline[rel]
	s.mu.RUnlock()

	now, exists, err := s.snapshotPath(rel)
	if err != nil {
		return protocol.ChangeDiffResultPayload{}, err
	}

	switch {
	case !hadBase && !exists:
		return protocol.ChangeDiffResultPayload{
			ExpectedHash: snapshotExpectedHash(now, exists),
			Path:         rel,
			Message:      "No diff is available for this path.",
		}, nil
	case hadBase && !exists:
		diff, truncated, binary := deletionDiff(base)
		return protocol.ChangeDiffResultPayload{
			ExpectedHash: snapshotExpectedHash(now, exists),
			Path:         rel,
			Diff:         diff,
			Truncated:    truncated,
			Binary:       binary,
			Restorable:   base.Available,
			Message:      emptyDiffMessage(diff, binary),
		}, nil
	case !hadBase && exists:
		diff, truncated, binary := additionDiff(now)
		return protocol.ChangeDiffResultPayload{
			ExpectedHash: snapshotExpectedHash(now, exists),
			Path:         rel,
			Diff:         diff,
			Truncated:    truncated,
			Binary:       binary,
			Restorable:   true,
			Message:      emptyDiffMessage(diff, binary),
		}, nil
	default:
		diff, truncated, binary := modifiedDiff(base, now)
		return protocol.ChangeDiffResultPayload{
			ExpectedHash: snapshotExpectedHash(now, exists),
			Path:         rel,
			Diff:         diff,
			Truncated:    truncated,
			Binary:       binary,
			Restorable:   base.Available,
			Message:      emptyDiffMessage(diff, binary),
		}, nil
	}
}
