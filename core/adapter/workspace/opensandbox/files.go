package opensandbox

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domainsandbox "myai/core/domain/sandbox"
	"myai/core/pathpolicy"
)

const maxSynchronizedFileBytes = int64(32 * 1024 * 1024)

type fileState struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
	Mode uint32 `json:"mode"`
}

func scanLocal(ctx context.Context, root string) (map[string]fileState, error) {
	directory, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	result := make(map[string]fileState)
	err = fs.WalkDir(directory.FS(), ".", func(relative string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if skipped(relative) || entry.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if err := rejectLinkedLocalPath(directory, relative, false); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := openLocalRegularFile(directory, relative)
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
		result[relative] = fileState{Path: relative, Hash: hex.EncodeToString(hasher.Sum(nil)), Size: size, Mode: uint32(info.Mode().Perm())}
		return nil
	})
	return result, err
}

func uploadChanged(ctx context.Context, remote remoteWorkspace, localRoot string, previous map[string]fileState, current map[string]fileState) error {
	directory, err := os.OpenRoot(localRoot)
	if err != nil {
		return err
	}
	defer directory.Close()
	deleted := make([]string, 0)
	for path := range previous {
		// A policy upgrade must not turn an old excluded entry into a delete.
		if skipped(path) {
			continue
		}
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
	for _, path := range sortedPaths(current) {
		if skipped(path) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		state := current[path]
		if before, exists := previous[path]; exists && before.Hash == state.Hash && before.Size == state.Size {
			continue
		}
		if state.Size < 0 || state.Size > maxSynchronizedFileBytes {
			return fmt.Errorf("file %s exceeds OpenSandbox synchronization limit of %d bytes", path, maxSynchronizedFileBytes)
		}
		file, err := openLocalRegularFile(directory, path)
		if err != nil {
			return err
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maxSynchronizedFileBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if int64(len(content)) > maxSynchronizedFileBytes {
			return fmt.Errorf("file %s exceeds OpenSandbox synchronization limit of %d bytes", path, maxSynchronizedFileBytes)
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
	if skipped(state.Path) {
		return fmt.Errorf("invalid or protected synchronized path %q", state.Path)
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := rejectLinkedLocalPath(directory, state.Path, true); err != nil {
		return err
	}
	parent := filepath.Dir(filepath.FromSlash(state.Path))
	if err := directory.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	// Open the actual parent once, then stage and replace only its direct child.
	// An ancestor rename cannot redirect the subsequent write outside this Root.
	parentRoot, err := directory.OpenRoot(parent)
	if err != nil {
		return err
	}
	defer parentRoot.Close()
	name := filepath.Base(state.Path)
	checkTarget := func() error {
		if err := rejectLinkedLocalPath(parentRoot, name, true); err != nil {
			return err
		}
		info, err := parentRoot.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("synchronized path is not a regular file: %s", state.Path)
		}
		return nil
	}
	if err := checkTarget(); err != nil {
		return err
	}
	var temporary *os.File
	var temporaryName string
	for attempts := 0; attempts < 10; attempts++ {
		temporaryName = ".opensandbox-" + rand.Text()
		temporary, err = parentRoot.OpenFile(temporaryName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if !errors.Is(err, os.ErrExist) {
			break
		}
	}
	if err != nil {
		return err
	}
	defer parentRoot.Remove(temporaryName)
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
	if err := checkTarget(); err != nil {
		return err
	}
	if err := parentRoot.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return parentRoot.Rename(temporaryName, name)
}

func removeLocalFile(root string, relative string) error {
	if skipped(relative) {
		return fmt.Errorf("invalid or protected synchronized path %q", relative)
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := rejectLinkedLocalPath(directory, relative, false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	info, err := directory.Lstat(filepath.FromSlash(relative))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("synchronized path is not a regular file: %s", relative)
	}
	err = directory.Remove(filepath.FromSlash(relative))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func safeLocalPath(root string, relative string) (string, error) {
	if skipped(relative) {
		return "", fmt.Errorf("invalid or protected synchronized path %q", relative)
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer directory.Close()
	if err := rejectLinkedLocalPath(directory, relative, true); err != nil {
		return "", err
	}
	return pathpolicy.Resolve(root, relative)
}

func skipped(relative string) bool {
	return !validSynchronizedPath(relative) || pathpolicy.ShouldSkipSnapshot(relative) || pathpolicy.IsSensitive(relative)
}

func validSynchronizedPath(path string) bool {
	if !fs.ValidPath(path) || path == "." || strings.ContainsAny(path, "\\:") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if strings.TrimRight(part, ". ") != part {
			return false
		}
	}
	return true
}

func rejectLinkedLocalPath(root *os.Root, relative string, allowMissing bool) error {
	if !validSynchronizedPath(relative) {
		return fmt.Errorf("invalid synchronized path %q", relative)
	}
	parts := strings.Split(relative, "/")
	for index := range parts {
		info, err := root.Lstat(filepath.FromSlash(strings.Join(parts[:index+1], "/")))
		if allowMissing && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("synchronized path is a link or non-regular file: %s", relative)
		}
	}
	return nil
}

func openLocalRegularFile(root *os.Root, relative string) (*os.File, error) {
	if err := rejectLinkedLocalPath(root, relative, false); err != nil {
		return nil, err
	}
	before, err := root.Lstat(filepath.FromSlash(relative))
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("synchronized path is not a regular file: %s", relative)
	}
	file, err := root.Open(filepath.FromSlash(relative))
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("synchronized file changed during open: %s", relative)
	}
	return file, nil
}

func sortedPaths(values map[string]fileState) []string {
	paths := make([]string, 0, len(values))
	for path := range values {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
