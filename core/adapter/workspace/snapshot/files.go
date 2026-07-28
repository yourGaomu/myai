package snapshot

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
)

var ignoredNames = map[string]bool{
	".git": true, ".idea": true, ".expo": true, ".next": true, ".cache": true, ".myai": true,
	"node_modules": true, "dist": true, "build": true,
}

var sensitiveNames = map[string]bool{
	".env": true, ".env.local": true, ".env.production": true,
	"id_rsa": true, "id_ed25519": true,
}

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
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		name := strings.ToLower(part)
		if ignoredNames[name] || sensitiveNames[name] {
			return true
		}
	}
	return false
}

func copyWorkspace(ctx context.Context, source string, target string, excludedRoots ...string) ([]fileState, error) {
	files := make([]fileState, 0)
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == source {
			return nil
		}
		if isWithinAnyRoot(path, excludedRoots) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if shouldSkip(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		state, err := copyFileWithHash(ctx, path, destination, info.Mode().Perm())
		if err != nil {
			return err
		}
		state.Path = filepath.ToSlash(relative)
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
		if shouldSkip(relative) {
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
		state, err := inspectFile(path)
		if err != nil {
			return err
		}
		state.Path = filepath.ToSlash(relative)
		result[state.Path] = state
		return nil
	})
	return result, err
}

func inspectWorkspaceFile(root string, relative string) (fileState, bool, error) {
	abs, err := safeJoin(root, relative)
	if err != nil {
		return fileState{}, false, err
	}
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return fileState{}, false, nil
	}
	if err != nil {
		return fileState{}, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fileState{}, false, fmt.Errorf("workspace path is not a regular file: %s", relative)
	}
	state, err := inspectFile(abs)
	state.Path = filepath.ToSlash(relative)
	return state, true, err
}

func inspectFile(path string) (fileState, error) {
	file, err := os.Open(path)
	if err != nil {
		return fileState{}, err
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
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
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fileState{}, err
	}
	input, err := os.Open(source)
	if err != nil {
		return fileState{}, err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
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
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("invalid workspace-relative path %q", relative)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	abs := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	abs, err = resolveExistingPath(abs)
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(root, abs)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
		return "", fmt.Errorf("path escapes workspace: %s", relative)
	}
	return abs, nil
}

func resolveExistingPath(path string) (string, error) {
	current := filepath.Clean(path)
	missing := make([]string, 0, 4)
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Abs(filepath.Clean(resolved))
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
