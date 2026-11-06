package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	domainhistory "myai/core/domain/history"
)

const maxRecordedFileBytes = 512 * 1024

func SnapshotFile(abs string, rel string) (domainhistory.FileSnapshot, bool, error) {
	if shouldSkipSnapshotPath(rel) {
		return domainhistory.FileSnapshot{}, false, nil
	}
	// Compatibility entry point: bind the absolute path to the workspace encoded
	// by rel, then use the same Root reader as recorders and workspace scans.
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/"))))
	if rel == "." || !filepath.IsLocal(filepath.FromSlash(rel)) {
		return domainhistory.FileSnapshot{}, false, errors.New("invalid snapshot path")
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	workspace := abs
	for range strings.Split(rel, "/") {
		workspace = filepath.Dir(workspace)
	}
	if !sameSnapshotPath(filepath.Join(workspace, filepath.FromSlash(rel)), abs) {
		return domainhistory.FileSnapshot{}, false, errors.New("snapshot path does not match its workspace")
	}
	return snapshotWorkspacePath(workspace, rel)
}

func sameSnapshotPath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b) || (os.PathSeparator == '\\' && strings.EqualFold(filepath.Clean(a), filepath.Clean(b)))
}

func snapshotWorkspacePath(workspace, rel string) (domainhistory.FileSnapshot, bool, error) {
	if shouldSkipSnapshotPath(rel) {
		return domainhistory.FileSnapshot{}, false, nil
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	defer root.Close()
	return snapshotRootPath(root, rel)
}

func validateSnapshotPath(root *os.Root, rel string) error {
	if rel == "." || !filepath.IsLocal(filepath.FromSlash(rel)) {
		return errors.New("invalid snapshot path")
	}
	prefix := ""
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if strings.ContainsAny(part, ":\x00") || (part != "." && part != ".." && strings.TrimRight(part, ". ") != part) {
			return errors.New("ambiguous snapshot path")
		}
		prefix = path.Join(prefix, part)
		info, err := root.Lstat(prefix)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 ||
			(!info.IsDir() && !info.Mode().IsRegular()) || (i < len(parts)-1 && !info.IsDir()) {
			return errors.New("snapshot path contains a link or non-regular entry")
		}
	}
	return nil
}

func snapshotRootPath(root *os.Root, rel string) (domainhistory.FileSnapshot, bool, error) {
	if shouldSkipSnapshotPath(rel) {
		return domainhistory.FileSnapshot{}, false, nil
	}
	if err := validateSnapshotPath(root, rel); err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	info, err := root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return domainhistory.FileSnapshot{}, false, nil
	}
	if err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	if !info.Mode().IsRegular() {
		return domainhistory.FileSnapshot{}, false, errors.New("cannot snapshot a non-regular file")
	}
	file, err := root.Open(rel)
	if err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return domainhistory.FileSnapshot{}, false, errors.New("snapshot file changed while opening")
	}
	content, digest, err := readSnapshotContent(file, info.Size())
	if err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	finalInfo, err := file.Stat()
	if err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	if finalInfo.Size() != info.Size() || !finalInfo.ModTime().Equal(info.ModTime()) {
		return domainhistory.FileSnapshot{}, false, errors.New("snapshot file changed while reading")
	}
	if err := validateSnapshotPath(root, rel); err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	snapshot := domainhistory.FileSnapshot{
		Path: rel, Size: info.Size(), Hash: digest, Binary: isBinary(content),
		TooLarge: info.Size() > int64(maxRecordedFileBytes), Mode: info.Mode(),
	}
	snapshot.Available = !snapshot.Binary && !snapshot.TooLarge
	if snapshot.Available {
		snapshot.Content = append([]byte(nil), content...)
	}
	return snapshot, true, nil
}

func snapshotWorkspace(ctx context.Context, workspace string) (map[string]domainhistory.FileSnapshot, error) {
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	result := make(map[string]domainhistory.FileSnapshot)
	err = fs.WalkDir(root.FS(), ".", func(rel string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if shouldSkipSnapshotPath(rel) || entry.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		snapshot, exists, err := snapshotRootPath(root, rel)
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

func snapshotChanged(before *domainhistory.FileSnapshot, after *domainhistory.FileSnapshot) bool {
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

func changeType(before *domainhistory.FileSnapshot, after *domainhistory.FileSnapshot) string {
	switch {
	case before == nil && after != nil:
		return "added"
	case before != nil && after == nil:
		return "deleted"
	default:
		return "modified"
	}
}

func cloneSnapshotPtr(source *domainhistory.FileSnapshot) *domainhistory.FileSnapshot {
	if source == nil {
		return nil
	}
	copied := *source
	if source.Content != nil {
		copied.Content = append([]byte(nil), source.Content...)
	}
	return &copied
}

func cloneFileChange(source domainhistory.FileChange) domainhistory.FileChange {
	return domainhistory.FileChange{
		Path:       source.Path,
		ChangeType: source.ChangeType,
		Before:     cloneSnapshotPtr(source.Before),
		After:      cloneSnapshotPtr(source.After),
		CreatedAt:  source.CreatedAt,
	}
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
