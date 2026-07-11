package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	domainhistory "myai/core/domain/history"
)

const maxRecordedFileBytes = 512 * 1024

func SnapshotFile(abs string, rel string) (domainhistory.FileSnapshot, bool, error) {
	// 二进制或超大文件只记录元数据和哈希，不保存可恢复内容，防止 SQLite 无限制膨胀。
	info, err := os.Stat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return domainhistory.FileSnapshot{}, false, nil
	}
	if err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	if info.IsDir() {
		return domainhistory.FileSnapshot{}, false, nil
	}

	file, err := os.Open(abs)
	if err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}
	defer file.Close()

	content, digest, err := readSnapshotContent(file, info.Size())
	if err != nil {
		return domainhistory.FileSnapshot{}, false, err
	}

	snapshot := domainhistory.FileSnapshot{
		Path:      filepath.ToSlash(rel),
		Size:      info.Size(),
		Hash:      digest,
		Binary:    isBinary(content),
		TooLarge:  info.Size() > int64(maxRecordedFileBytes),
		Mode:      info.Mode(),
		Available: false,
	}
	snapshot.Available = !snapshot.Binary && !snapshot.TooLarge
	if snapshot.Available {
		snapshot.Content = append([]byte(nil), content...)
	}
	return snapshot, true, nil
}

func snapshotWorkspace(ctx context.Context, workspace string) (map[string]domainhistory.FileSnapshot, error) {
	// 全量快照跳过依赖目录、版本库和敏感文件；符号链接目标必须仍在 workspace 内。
	result := make(map[string]domainhistory.FileSnapshot)
	err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == workspace {
			return nil
		}

		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if shouldSkipSnapshotPath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err != nil || !insideRoot(workspace, target) {
				return nil
			}
			path = target
		}

		snapshot, exists, err := SnapshotFile(path, rel)
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
