package changes

import (
	"context"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

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
	err := fs.WalkDir(s.files.FS(), ".", func(rel string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if shouldHidePath(rel) || entry.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if err := s.validatePath(rel); err != nil {
				if errors.Is(err, errUnsafePath) {
					return fs.SkipDir
				}
				return err
			}
			return nil
		}
		item, exists, err := s.snapshotPath(rel)
		if errors.Is(err, errUnsafePath) {
			return nil
		}
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

// Validate every existing component, including Windows junctions (ModeIrregular).
// Root operations remain the containment boundary if a component changes later.
func (s *Service) validatePath(rel string) error {
	return validateRootPath(s.files, rel)
}

func validateRootPath(root *os.Root, rel string) error {
	current := ""
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 ||
			(!info.IsDir() && !info.Mode().IsRegular()) ||
			(i < len(parts)-1 && !info.IsDir()) {
			return unsafePath(rel)
		}
	}
	return nil
}

func (s *Service) snapshotPath(rel string) (snapshotEntry, bool, error) {
	if err := s.validatePath(rel); err != nil {
		return snapshotEntry{}, false, err
	}
	linkInfo, err := s.files.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return snapshotEntry{}, false, nil
	}
	if err != nil {
		return snapshotEntry{}, false, err
	}
	if !linkInfo.Mode().IsRegular() {
		return snapshotEntry{}, false, unsafePath(rel)
	}
	file, err := s.files.Open(rel)
	if err != nil {
		return snapshotEntry{}, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return snapshotEntry{}, false, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(linkInfo, info) {
		return snapshotEntry{}, false, unsafePath(rel)
	}
	content, digest, err := readSnapshotContent(file, info.Size())
	if err != nil {
		return snapshotEntry{}, false, err
	}
	finalInfo, err := file.Stat()
	if err != nil {
		return snapshotEntry{}, false, err
	}
	if finalInfo.Size() != info.Size() || !finalInfo.ModTime().Equal(info.ModTime()) {
		return snapshotEntry{}, false, errors.New("workspace file changed while reading")
	}
	if err := s.validatePath(rel); err != nil {
		return snapshotEntry{}, false, err
	}
	entry := snapshotEntry{
		Path: rel, Size: info.Size(), Mode: info.Mode(), Hash: digest,
		Binary: isBinary(content), TooLarge: info.Size() > int64(maxSnapshotBytes),
	}
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
		if shouldHidePath(key) || shouldHidePath(value.Path) {
			continue
		}
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
