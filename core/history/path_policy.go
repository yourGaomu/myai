package history

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myai/core/pathpolicy"
)

func cleanWorkspace(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = "."
	}
	abs, err := filepath.Abs(workspace)
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

func cleanRecorderPath(workspace string, path string) (string, string, error) {
	// 所有记录路径先解析为绝对路径并验证位于 workspace，避免历史系统读取外部文件。
	path = strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	if path == "" || strings.ContainsRune(path, '\x00') {
		return "", "", errors.New("path is empty or invalid")
	}
	path = filepath.FromSlash(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", "", err
	}
	if !insideRoot(workspace, abs) {
		return "", "", fmt.Errorf("path is outside workspace: %s", path)
	}
	rel, err := filepath.Rel(workspace, abs)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(rel), abs, nil
}

func shouldSkipSnapshotPath(path string) bool {
	return pathpolicy.ShouldSkipSnapshot(path)
}

func insideRoot(root string, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
