package changes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	domainhistory "myai/core/domain/history"
	"myai/core/remote/protocol"
)

func snapshotExpectedHash(now snapshotEntry, exists bool) string {
	if !exists {
		return "missing"
	}
	return hex.EncodeToString(now.Hash[:])
}

func (s *Service) Revert(ctx context.Context, payload protocol.ChangeRevertPayload) (protocol.ChangeRevertResultPayload, error) {
	s.operation.Lock()
	defer s.operation.Unlock()
	rel, _, err := cleanPath(s.root, payload.Path)
	if err != nil {
		return protocol.ChangeRevertResultPayload{}, err
	}
	if shouldHidePath(rel) {
		return protocol.ChangeRevertResultPayload{}, errors.New("refusing to revert a protected workspace path")
	}
	if payload.ExpectedHash == "" {
		return protocol.ChangeRevertResultPayload{}, errors.New("expected_hash is required; preview this change again")
	}
	if err := ctx.Err(); err != nil {
		return protocol.ChangeRevertResultPayload{}, err
	}
	now, exists, err := s.snapshotPath(rel)
	if err != nil {
		return protocol.ChangeRevertResultPayload{}, err
	}
	if payload.ExpectedHash != snapshotExpectedHash(now, exists) {
		return protocol.ChangeRevertResultPayload{}, fmt.Errorf("workspace conflict: %s changed since preview", rel)
	}
	s.mu.RLock()
	base, hasBase := s.baseline[rel]
	s.mu.RUnlock()
	if !hasBase && !exists {
		return protocol.ChangeRevertResultPayload{}, fmt.Errorf("new file no longer exists: %s", rel)
	}
	plan := restorePlan{path: rel, current: snapshotPtr(now, exists)}
	if hasBase {
		plan.desired = snapshotPtr(base, true)
		if err := validateRestorable(plan.desired); err != nil {
			return protocol.ChangeRevertResultPayload{}, err
		}
	}
	if _, err := s.restore(ctx, []restorePlan{plan}, "baseline"); err != nil {
		return protocol.ChangeRevertResultPayload{}, err
	}
	return protocol.ChangeRevertResultPayload{
		Path: rel, Reverted: true, Message: "File restored to the SQLite workspace history baseline.",
	}, nil
}

func (s *Service) RevertCheckpoint(ctx context.Context, payload protocol.HistoryRevertPayload) (protocol.HistoryRevertResultPayload, error) {
	s.operation.Lock()
	defer s.operation.Unlock()
	changes, err := s.store.LoadCheckpointChanges(ctx, s.root, payload.CheckpointID)
	if err != nil {
		return protocol.HistoryRevertResultPayload{}, err
	}
	if len(changes) == 0 {
		return protocol.HistoryRevertResultPayload{}, fmt.Errorf("checkpoint has no file changes: %s", payload.CheckpointID)
	}
	plans := make([]restorePlan, 0, len(changes))
	seen := make(map[string]bool, len(changes))
	// Validate the entire batch before creating backups or changing workspace files.
	for i := len(changes) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return protocol.HistoryRevertResultPayload{}, err
		}
		change := changes[i]
		rel, _, err := cleanPath(s.root, change.Path)
		if err != nil {
			return protocol.HistoryRevertResultPayload{}, err
		}
		if shouldHidePath(rel) {
			return protocol.HistoryRevertResultPayload{}, errors.New("checkpoint contains a protected workspace path")
		}
		key := filesystemPathKey(rel)
		if seen[key] {
			return protocol.HistoryRevertResultPayload{}, fmt.Errorf("duplicate checkpoint path: %s", rel)
		}
		seen[key] = true
		if change.Before == nil && change.After == nil {
			return protocol.HistoryRevertResultPayload{}, fmt.Errorf("checkpoint has no file state: %s", rel)
		}
		if err := validateRestorable(change.Before); err != nil {
			return protocol.HistoryRevertResultPayload{}, err
		}
		now, exists, err := s.snapshotPath(rel)
		if err != nil {
			return protocol.HistoryRevertResultPayload{}, err
		}
		if !snapshotMatches(snapshotPtr(now, exists), change.After) {
			return protocol.HistoryRevertResultPayload{}, fmt.Errorf("workspace conflict: %s does not match checkpoint after state", rel)
		}
		plans = append(plans, restorePlan{path: rel, current: snapshotPtr(now, exists), desired: change.Before})
	}
	inverseID, err := s.restore(ctx, plans, payload.CheckpointID)
	if err != nil {
		return protocol.HistoryRevertResultPayload{}, err
	}
	paths := make([]string, 0, len(plans))
	for _, plan := range plans {
		paths = append(paths, plan.path)
	}
	return protocol.HistoryRevertResultPayload{
		CheckpointID: payload.CheckpointID, InverseCheckpointID: inverseID,
		Reverted: true, Paths: paths, Message: fmt.Sprintf("Reverted checkpoint %s.", payload.CheckpointID),
	}, nil
}

func snapshotPtr(value snapshotEntry, exists bool) *domainhistory.FileSnapshot {
	if !exists {
		return nil
	}
	return &domainhistory.FileSnapshot{
		Path: value.Path, Hash: value.Hash, Size: value.Size, Mode: value.Mode,
		Content: append([]byte(nil), value.Content...), Available: value.Available,
		Binary: value.Binary, TooLarge: value.TooLarge,
	}
}

func snapshotMatches(actual, expected *domainhistory.FileSnapshot) bool {
	if actual == nil || expected == nil {
		return actual == nil && expected == nil
	}
	return actual.Hash == expected.Hash && actual.Size == expected.Size && actual.Mode.Perm() == expected.Mode.Perm()
}

func validateRestorable(file *domainhistory.FileSnapshot) error {
	if file == nil {
		return nil
	}
	if !file.Available || file.Binary || file.TooLarge || file.Mode&(os.ModeType) != 0 ||
		int64(len(file.Content)) != file.Size || sha256.Sum256(file.Content) != file.Hash {
		return fmt.Errorf("checkpoint content is not available or valid for: %s", file.Path)
	}
	return nil
}
