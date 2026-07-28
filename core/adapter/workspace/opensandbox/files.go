package opensandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domainsandbox "myai/core/domain/sandbox"
)

const maxSynchronizedFileBytes = int64(32 * 1024 * 1024)

var ignoredNames = map[string]bool{
	".git": true, ".idea": true, ".expo": true, ".next": true, ".cache": true,
	"node_modules": true, "dist": true, "build": true,
}

type fileState struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
	Mode uint32 `json:"mode"`
}

func scanLocal(ctx context.Context, root string) (map[string]fileState, error) {
	result := make(map[string]fileState)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if skipped(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hasher := sha256.New()
		size, copyErr := io.Copy(hasher, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		relative = filepath.ToSlash(relative)
		result[relative] = fileState{Path: relative, Hash: hex.EncodeToString(hasher.Sum(nil)), Size: size, Mode: uint32(info.Mode().Perm())}
		return nil
	})
	return result, err
}

func uploadChanged(ctx context.Context, remote remoteWorkspace, localRoot string, previous map[string]fileState, current map[string]fileState) error {
	deleted := make([]string, 0)
	for path := range previous {
		if _, exists := current[path]; !exists {
			deleted = append(deleted, path)
		}
	}
	sort.Strings(deleted)
	if len(deleted) > 0 {
		if err := deleteRemoteFiles(ctx, remote, deleted); err != nil {
			return err
		}
	}

	batch := make([]domainsandbox.File, 0, 32)
	batchBytes := int64(0)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := remote.UploadFiles(ctx, batch); err != nil {
			return err
		}
		batch = batch[:0]
		batchBytes = 0
		return nil
	}
	paths := sortedPaths(current)
	for _, path := range paths {
		state := current[path]
		if before, exists := previous[path]; exists && before.Hash == state.Hash && before.Size == state.Size {
			continue
		}
		if state.Size > maxSynchronizedFileBytes {
			return fmt.Errorf("file %s exceeds OpenSandbox synchronization limit of %d bytes", path, maxSynchronizedFileBytes)
		}
		content, err := os.ReadFile(filepath.Join(localRoot, filepath.FromSlash(path)))
		if err != nil {
			return err
		}
		if batchBytes+int64(len(content)) > maxSynchronizedFileBytes && len(batch) > 0 {
			if err := flush(); err != nil {
				return err
			}
		}
		batch = append(batch, domainsandbox.File{Path: "/workspace/" + path, Content: content, Mode: state.Mode})
		batchBytes += int64(len(content))
	}
	return flush()
}

func writeLocalFile(root string, state fileState, content []byte) error {
	path, err := safeLocalPath(root, state.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".opensandbox-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(os.FileMode(state.Mode).Perm()); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func safeLocalPath(root string, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("invalid synchronized path %q", relative)
	}
	path := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("synchronized path escapes workspace: %s", relative)
	}
	return path, nil
}

func skipped(relative string) bool {
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if ignoredNames[strings.ToLower(part)] {
			return true
		}
	}
	return false
}

func sortedPaths(values map[string]fileState) []string {
	paths := make([]string, 0, len(values))
	for path := range values {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
