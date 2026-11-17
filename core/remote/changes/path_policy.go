package changes

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"myai/core/pathpolicy"
)

func cleanPath(root string, path string) (string, string, error) {
	path = strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	if path == "" || strings.ContainsAny(path, ":\x00") {
		return "", "", errors.New("invalid change path")
	}
	for _, component := range strings.Split(path, "/") {
		if component != "." && component != ".." && strings.TrimRight(component, ". ") != component {
			return "", "", errors.New("ambiguous change path")
		}
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || !filepath.IsLocal(clean) {
		return "", "", errors.New("change path must be local to the workspace")
	}
	return filepath.ToSlash(clean), filepath.Join(root, clean), nil
}

func shouldHidePath(path string) bool {
	return pathpolicy.ShouldSkipSnapshot(path)
}

func isBinary(content []byte) bool {
	return bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content)
}

var errUnsafePath = errors.New("workspace path contains a link or non-regular entry")

func unsafePath(path string) error {
	return fmt.Errorf("%w: %s", errUnsafePath, path)
}
