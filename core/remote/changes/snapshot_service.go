package changes

import (
	"context"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"

	domainhistory "myai/core/domain/history"
)

const maxSnapshotBytes = 512 * 1024

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

func snapshotToHistory(source map[string]snapshotEntry) map[string]domainhistory.FileSnapshot {
	result := make(map[string]domainhistory.FileSnapshot, len(source))
	for key, value := range source {
		item := domainhistory.FileSnapshot{
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

func historyToSnapshot(source map[string]domainhistory.FileSnapshot) map[string]snapshotEntry {
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
