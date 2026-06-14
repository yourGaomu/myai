package changes

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
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"myai/core/history"
	"myai/core/remote/protocol"
)

const (
	defaultChangeLimit = 200
	maxChangeLimit     = 1000
	maxDiffBytes       = 256 * 1024
	maxSnapshotBytes   = 512 * 1024
	maxDiffLineProduct = 250000
)

var ignoredNames = map[string]bool{
	".git":         true,
	".idea":        true,
	".expo":        true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".cache":       true,
}

var sensitiveNames = map[string]bool{
	".env":            true,
	".env.local":      true,
	".env.production": true,
	"id_rsa":          true,
	"id_ed25519":      true,
}

type Service struct {
	root     string
	store    *history.SQLiteStore
	mu       sync.RWMutex
	baseline map[string]snapshotEntry
}

type snapshotEntry struct {
	Path      string
	Size      int64
	Hash      [32]byte
	Content   []byte
	Binary    bool
	TooLarge  bool
	Mode      os.FileMode
	Available bool
}

func New(root string) (*Service, error) {
	return NewWithHistoryPath(root, "")
}

func NewWithHistoryPath(root string, historyPath string) (*Service, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}

	abs, err := filepath.Abs(root)
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

	if strings.TrimSpace(historyPath) == "" {
		historyPath, err = history.DefaultSQLitePath(abs)
		if err != nil {
			return nil, err
		}
	}
	store, err := history.OpenSQLite(historyPath)
	if err != nil {
		return nil, err
	}

	service := &Service{
		root:     abs,
		store:    store,
		baseline: make(map[string]snapshotEntry),
	}
	if err := service.loadOrCreateBaseline(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}
	return service, nil
}

func (s *Service) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

func (s *Service) Reset(ctx context.Context) error {
	snapshot, err := s.scan(ctx)
	if err != nil {
		return err
	}
	if err := s.store.ReplaceBaseline(ctx, s.root, snapshotToHistory(snapshot)); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseline = snapshot
	return nil
}

func (s *Service) loadOrCreateBaseline(ctx context.Context) error {
	exists, err := s.store.HasBaseline(ctx, s.root)
	if err != nil {
		return err
	}
	if !exists {
		return s.Reset(ctx)
	}

	files, err := s.store.LoadBaseline(ctx, s.root)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseline = historyToSnapshot(files)
	return nil
}

func (s *Service) List(ctx context.Context, payload protocol.ChangesListPayload) (protocol.ChangesListResultPayload, error) {
	limit := payload.Limit
	if limit <= 0 {
		limit = defaultChangeLimit
	}
	if limit > maxChangeLimit {
		limit = maxChangeLimit
	}

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
			entries = append(entries, protocol.ChangeEntry{
				Path:       path,
				Status:     "deleted",
				Deleted:    true,
				Unstaged:   true,
				Restorable: base.Available,
			})
		}
	}
	for path, now := range current {
		base, ok := baseline[path]
		switch {
		case !ok:
			entries = append(entries, protocol.ChangeEntry{
				Path:       path,
				Status:     "added",
				Untracked:  true,
				Unstaged:   true,
				Restorable: true,
			})
		case base.Hash != now.Hash || base.Size != now.Size || base.Binary != now.Binary || base.TooLarge != now.TooLarge:
			entries = append(entries, protocol.ChangeEntry{
				Path:       path,
				Status:     "modified",
				Unstaged:   true,
				Restorable: base.Available,
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
	rel, abs, err := cleanPath(s.root, payload.Path)
	if err != nil {
		return protocol.ChangeDiffResultPayload{}, err
	}
	if shouldHidePath(rel) {
		return protocol.ChangeDiffResultPayload{}, fmt.Errorf("refusing to preview sensitive change: %s", rel)
	}

	s.mu.RLock()
	base, hadBase := s.baseline[rel]
	s.mu.RUnlock()

	now, exists, err := snapshotPath(s.root, abs, rel)
	if err != nil {
		return protocol.ChangeDiffResultPayload{}, err
	}

	switch {
	case !hadBase && !exists:
		return protocol.ChangeDiffResultPayload{
			Path:    rel,
			Message: "No diff is available for this path.",
		}, nil
	case hadBase && !exists:
		diff, truncated, binary := deletionDiff(base)
		return protocol.ChangeDiffResultPayload{
			Path:       rel,
			Diff:       diff,
			Truncated:  truncated,
			Binary:     binary,
			Restorable: base.Available,
			Message:    emptyDiffMessage(diff, binary),
		}, nil
	case !hadBase && exists:
		diff, truncated, binary := additionDiff(now)
		return protocol.ChangeDiffResultPayload{
			Path:       rel,
			Diff:       diff,
			Truncated:  truncated,
			Binary:     binary,
			Restorable: true,
			Message:    emptyDiffMessage(diff, binary),
		}, nil
	default:
		diff, truncated, binary := modifiedDiff(base, now)
		return protocol.ChangeDiffResultPayload{
			Path:       rel,
			Diff:       diff,
			Truncated:  truncated,
			Binary:     binary,
			Restorable: base.Available,
			Message:    emptyDiffMessage(diff, binary),
		}, nil
	}
}

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
		item := storedChangeDiff(change)
		files = append(files, item)
	}

	return protocol.HistoryDiffResultPayload{
		CheckpointID: payload.CheckpointID,
		Files:        files,
		Count:        len(files),
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

func storedChangeDiff(change history.StoredFileChange) protocol.HistoryFileDiff {
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

func (s *Service) revertStoredChange(ctx context.Context, change history.StoredFileChange) error {
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

func (s *Service) scan(ctx context.Context) (map[string]snapshotEntry, error) {
	result := make(map[string]snapshotEntry)
	err := filepath.WalkDir(s.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == s.root {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if shouldHidePath(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldHidePath(name) {
			return nil
		}

		rel, err := s.relative(path)
		if err != nil {
			return err
		}
		item, exists, err := snapshotPath(s.root, path, rel)
		if err != nil {
			return err
		}
		if exists {
			result[rel] = item
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func snapshotPath(root string, abs string, rel string) (snapshotEntry, bool, error) {
	linkInfo, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return snapshotEntry{}, false, nil
	}
	if err != nil {
		return snapshotEntry{}, false, err
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return snapshotEntry{}, false, nil
		}
		target = filepath.Clean(target)
		if !insideRoot(root, target) {
			return snapshotEntry{}, false, nil
		}
		abs = target
	}

	info, err := os.Stat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return snapshotEntry{}, false, nil
	}
	if err != nil {
		return snapshotEntry{}, false, err
	}
	if info.IsDir() {
		return snapshotEntry{}, false, nil
	}

	entry := snapshotEntry{
		Path: rel,
		Size: info.Size(),
		Mode: info.Mode(),
	}

	file, err := os.Open(abs)
	if err != nil {
		return snapshotEntry{}, false, err
	}
	defer file.Close()

	content, digest, err := readSnapshotContent(file, info.Size())
	if err != nil {
		return snapshotEntry{}, false, err
	}
	entry.Hash = digest
	entry.Binary = isBinary(content)
	entry.TooLarge = info.Size() > int64(maxSnapshotBytes)
	entry.Available = !entry.Binary && !entry.TooLarge
	if entry.Available {
		entry.Content = append([]byte(nil), content...)
	}
	return entry, true, nil
}

func readSnapshotContent(file *os.File, size int64) ([]byte, [32]byte, error) {
	hasher := sha256.New()
	limit := maxSnapshotBytes
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

func copySnapshot(source map[string]snapshotEntry) map[string]snapshotEntry {
	copied := make(map[string]snapshotEntry, len(source))
	for key, value := range source {
		if value.Content != nil {
			value.Content = append([]byte(nil), value.Content...)
		}
		copied[key] = value
	}
	return copied
}

func snapshotToHistory(source map[string]snapshotEntry) map[string]history.FileSnapshot {
	result := make(map[string]history.FileSnapshot, len(source))
	for key, value := range source {
		item := history.FileSnapshot{
			Path:      value.Path,
			Size:      value.Size,
			Hash:      value.Hash,
			Binary:    value.Binary,
			TooLarge:  value.TooLarge,
			Mode:      value.Mode,
			Available: value.Available,
		}
		if value.Content != nil {
			item.Content = append([]byte(nil), value.Content...)
		}
		result[key] = item
	}
	return result
}

func historyToSnapshot(source map[string]history.FileSnapshot) map[string]snapshotEntry {
	result := make(map[string]snapshotEntry, len(source))
	for key, value := range source {
		item := snapshotEntry{
			Path:      value.Path,
			Size:      value.Size,
			Hash:      value.Hash,
			Binary:    value.Binary,
			TooLarge:  value.TooLarge,
			Mode:      value.Mode,
			Available: value.Available,
		}
		if value.Content != nil {
			item.Content = append([]byte(nil), value.Content...)
		}
		result[key] = item
	}
	return result
}

func historyAdditionDiff(entry *history.FileSnapshot) (string, bool, bool) {
	if entry == nil {
		return "", false, false
	}
	return additionDiff(historyFileToSnapshot(*entry))
}

func historyDeletionDiff(entry *history.FileSnapshot) (string, bool, bool) {
	if entry == nil {
		return "", false, false
	}
	return deletionDiff(historyFileToSnapshot(*entry))
}

func historyModifiedDiff(base *history.FileSnapshot, now *history.FileSnapshot) (string, bool, bool) {
	if base == nil || now == nil {
		return "", false, false
	}
	return modifiedDiff(historyFileToSnapshot(*base), historyFileToSnapshot(*now))
}

func historyFileToSnapshot(value history.FileSnapshot) snapshotEntry {
	item := snapshotEntry{
		Path:      value.Path,
		Size:      value.Size,
		Hash:      value.Hash,
		Content:   value.Content,
		Binary:    value.Binary,
		TooLarge:  value.TooLarge,
		Mode:      value.Mode,
		Available: value.Available,
	}
	if value.Content != nil {
		item.Content = append([]byte(nil), value.Content...)
	}
	return item
}

func additionDiff(entry snapshotEntry) (string, bool, bool) {
	if entry.Binary || entry.TooLarge {
		return "", entry.TooLarge, true
	}
	diff, truncated := fileDiff("", string(entry.Content), entry.Path)
	return diff, truncated, false
}

func deletionDiff(entry snapshotEntry) (string, bool, bool) {
	if entry.Binary || entry.TooLarge || !entry.Available {
		return "", entry.TooLarge, true
	}
	diff, truncated := fileDiff(string(entry.Content), "", entry.Path)
	return diff, truncated, false
}

func modifiedDiff(base snapshotEntry, now snapshotEntry) (string, bool, bool) {
	if base.Binary || now.Binary || base.TooLarge || now.TooLarge || !base.Available || !now.Available {
		return "", base.TooLarge || now.TooLarge, true
	}
	diff, truncated := fileDiff(string(base.Content), string(now.Content), now.Path)
	return diff, truncated, false
}

func fileDiff(oldText string, newText string, path string) (string, bool) {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)
	truncated := false

	var builder strings.Builder
	builder.WriteString("diff --myai a/")
	builder.WriteString(path)
	builder.WriteString(" b/")
	builder.WriteString(path)
	builder.WriteString("\n--- a/")
	builder.WriteString(path)
	builder.WriteString("\n+++ b/")
	builder.WriteString(path)
	builder.WriteString(fmt.Sprintf("\n@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines)))

	var ops []lineOp
	if len(oldLines)*len(newLines) > maxDiffLineProduct {
		truncated = true
		ops = compactDiffLines(oldLines, newLines)
	} else {
		ops = diffLines(oldLines, newLines)
	}
	for _, op := range ops {
		switch op.kind {
		case "equal":
			builder.WriteString(" ")
		case "delete":
			builder.WriteString("-")
		case "insert":
			builder.WriteString("+")
		}
		builder.WriteString(op.text)
		if !strings.HasSuffix(op.text, "\n") {
			builder.WriteString("\n")
		}
	}

	diff := builder.String()
	if len(diff) > maxDiffBytes {
		return diff[:maxDiffBytes], true
	}
	return diff, truncated
}

type lineOp struct {
	kind string
	text string
}

func diffLines(oldLines []string, newLines []string) []lineOp {
	m, n := len(oldLines), len(newLines)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	ops := make([]lineOp, 0, m+n)
	i, j := 0, 0
	for i < m && j < n {
		if oldLines[i] == newLines[j] {
			ops = append(ops, lineOp{kind: "equal", text: oldLines[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, lineOp{kind: "delete", text: oldLines[i]})
			i++
		} else {
			ops = append(ops, lineOp{kind: "insert", text: newLines[j]})
			j++
		}
	}
	for i < m {
		ops = append(ops, lineOp{kind: "delete", text: oldLines[i]})
		i++
	}
	for j < n {
		ops = append(ops, lineOp{kind: "insert", text: newLines[j]})
		j++
	}
	return ops
}

func compactDiffLines(oldLines []string, newLines []string) []lineOp {
	ops := make([]lineOp, 0, len(oldLines)+len(newLines))
	for _, line := range oldLines {
		ops = append(ops, lineOp{kind: "delete", text: line})
	}
	for _, line := range newLines {
		ops = append(ops, lineOp{kind: "insert", text: line})
	}
	return ops
}

func splitLines(text string) []string {
	if text == "" {
		return []string{}
	}
	return strings.SplitAfter(text, "\n")
}

func emptyDiffMessage(diff string, binary bool) string {
	if binary {
		return "Binary or oversized file diff is not available."
	}
	if strings.TrimSpace(diff) == "" {
		return "No diff is available for this path."
	}
	return ""
}

func cleanPath(root string, path string) (string, string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return "", "", errors.New("change path is empty")
	}
	if filepath.IsAbs(clean) {
		return "", "", errors.New("absolute paths are not allowed")
	}

	abs := filepath.Join(root, clean)
	abs = filepath.Clean(abs)
	if !insideRoot(root, abs) {
		return "", "", fmt.Errorf("path escapes workspace: %s", path)
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(rel), abs, nil
}

func (s *Service) relative(abs string) (string, error) {
	rel, err := filepath.Rel(s.root, abs)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return ".", nil
	}
	return filepath.ToSlash(rel), nil
}

func insideRoot(root string, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func shouldHidePath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		lower := strings.ToLower(part)
		if ignoredNames[lower] || sensitiveNames[lower] {
			return true
		}
	}
	return false
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
