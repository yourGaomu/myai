package snapshot

import (
	"context"
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

	"myai/core/pathpolicy"
)

func normalizeDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("workspace directory is empty")
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", abs)
	}
	return abs, nil
}

func shouldSkip(path string) bool {
	return !validRelativePath(path) || pathpolicy.ShouldSkipSnapshot(path)
}

func validRelativePath(path string) bool {
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

func copyWorkspace(ctx context.Context, source string, target string, excludedRoots ...string) ([]fileState, error) {
	sourceRoot, err := os.OpenRoot(source)
	if err != nil {
		return nil, err
	}
	defer sourceRoot.Close()
	targetRoot, err := os.OpenRoot(target)
	if err != nil {
		return nil, err
	}
	defer targetRoot.Close()
	files := make([]fileState, 0)
	err = fs.WalkDir(sourceRoot.FS(), ".", func(relative string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if shouldSkip(relative) || isWithinAnyRoot(filepath.Join(source, filepath.FromSlash(relative)), excludedRoots) || entry.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if err := rejectLinkedPath(sourceRoot, relative, false); err != nil {
			return err
		}
		if entry.IsDir() {
			if err := rejectLinkedPath(targetRoot, relative, true); err != nil {
				return err
			}
			return targetRoot.MkdirAll(filepath.FromSlash(relative), 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		state, err := copyRootedFileWithHash(ctx, sourceRoot, relative, targetRoot, relative, info.Mode().Perm())
		if err != nil {
			return err
		}
		state.Path = relative
		files = append(files, state)
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func isWithinAnyRoot(path string, roots []string) bool {
	path = filepath.Clean(path)
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		relative, err := filepath.Rel(filepath.Clean(root), path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func scanWorkspace(ctx context.Context, root string) (map[string]fileState, error) {
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
		if shouldSkip(relative) || entry.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if err := rejectLinkedPath(directory, relative, false); err != nil {
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
		state, err := inspectRootedFile(ctx, directory, relative)
		if err != nil {
			return err
		}
		state.Path = relative
		result[relative] = state
		return nil
	})
	return result, err
}

func inspectWorkspaceFile(root string, relative string) (fileState, bool, error) {
	if shouldSkip(relative) {
		return fileState{}, false, fmt.Errorf("invalid or protected workspace path: %s", relative)
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		return fileState{}, false, err
	}
	defer directory.Close()
	state, err := inspectRootedFile(context.Background(), directory, relative)
	if errors.Is(err, os.ErrNotExist) {
		return fileState{}, false, nil
	}
	state.Path = relative
	return state, err == nil, err
}

// Check every component, including junctions/reparse points reported as
// irregular files on Windows. Root also confines the actual I/O if a path is
// replaced between this check and the subsequent open.
func rejectLinkedPath(root *os.Root, relative string, allowMissing bool) error {
	if !validRelativePath(relative) {
		return fmt.Errorf("invalid workspace-relative path %q", relative)
	}
	parts := strings.Split(relative, "/")
	for index := range parts {
		name := filepath.FromSlash(strings.Join(parts[:index+1], "/"))
		info, err := root.Lstat(name)
		if allowMissing && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("workspace path is a link or non-regular file: %s", relative)
		}
	}
	return nil
}

func openRegularFile(root *os.Root, relative string) (*os.File, error) {
	if err := rejectLinkedPath(root, relative, false); err != nil {
		return nil, err
	}
	before, err := root.Lstat(filepath.FromSlash(relative))
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("workspace path is not a regular file: %s", relative)
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
		return nil, fmt.Errorf("workspace file changed during open: %s", relative)
	}
	return file, nil
}

func inspectRootedFile(ctx context.Context, root *os.Root, relative string) (fileState, error) {
	file, err := openRegularFile(root, relative)
	if err != nil {
		return fileState{}, err
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := copyWithContext(ctx, hasher, file)
	if err != nil {
		return fileState{}, err
	}
	info, err := file.Stat()
	if err != nil {
		return fileState{}, err
	}
	return fileState{Hash: hex.EncodeToString(hasher.Sum(nil)), Size: size, Mode: uint32(info.Mode().Perm())}, nil
}

func copyFileWithHash(ctx context.Context, source string, destination string, mode os.FileMode) (fileState, error) {
	sourceRoot, err := os.OpenRoot(filepath.Dir(source))
	if err != nil {
		return fileState{}, err
	}
	defer sourceRoot.Close()
	// The workspace copy uses its already-open destination Root directly. This
	// wrapper also serves the existing apply/rollback code's sibling temp files.
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fileState{}, err
	}
	targetRoot, err := os.OpenRoot(filepath.Dir(destination))
	if err != nil {
		return fileState{}, err
	}
	defer targetRoot.Close()
	return copyRootedFileWithHash(ctx, sourceRoot, filepath.Base(source), targetRoot, filepath.Base(destination), mode)
}

func copyRootedFileWithHash(ctx context.Context, sourceRoot *os.Root, source string, targetRoot *os.Root, destination string, mode os.FileMode) (fileState, error) {
	input, err := openRegularFile(sourceRoot, source)
	if err != nil {
		return fileState{}, err
	}
	defer input.Close()
	if err := rejectLinkedPath(targetRoot, destination, true); err != nil {
		return fileState{}, err
	}
	if err := targetRoot.MkdirAll(filepath.Dir(filepath.FromSlash(destination)), 0o755); err != nil {
		return fileState{}, err
	}
	output, err := targetRoot.OpenFile(filepath.FromSlash(destination), os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fileState{}, err
	}
	hasher := sha256.New()
	written, copyErr := copyWithContext(ctx, io.MultiWriter(output, hasher), input)
	closeErr := output.Close()
	if copyErr != nil {
		return fileState{}, copyErr
	}
	if closeErr != nil {
		return fileState{}, closeErr
	}
	return fileState{Hash: hex.EncodeToString(hasher.Sum(nil)), Size: written, Mode: uint32(mode.Perm())}, nil
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 128*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			written, writeErr := destination.Write(buffer[:read])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != read {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

func safeJoin(root string, relative string) (string, error) {
	if shouldSkip(relative) {
		return "", fmt.Errorf("invalid or protected workspace-relative path %q", relative)
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer directory.Close()
	if err := rejectLinkedPath(directory, relative, true); err != nil {
		return "", err
	}
	return pathpolicy.Resolve(root, relative)
}
