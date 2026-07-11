package changes

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var ignoredNames = map[string]bool{
	".git":         true,
	".idea":        true,
	".expo":        true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".cache":       true,
}

var sensitiveNames = map[string]bool{
	".env":            true,
	".env.local":      true,
	".env.production": true,
	"id_rsa":          true,
	"id_ed25519":      true,
}

func cleanPath(root string, path string) (string, string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return "", "", errors.New("change path is empty")
	}
	if filepath.IsAbs(clean) {
		return "", "", errors.New("absolute paths are not allowed")
	}

	abs := filepath.Join(root, clean)
	abs = filepath.Clean(abs)
	if !insideRoot(root, abs) {
		return "", "", fmt.Errorf("path escapes workspace: %s", path)
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(rel), abs, nil
}

func (s *Service) relative(abs string) (string, error) {
	rel, err := filepath.Rel(s.root, abs)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return ".", nil
	}
	return filepath.ToSlash(rel), nil
}

func insideRoot(root string, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func shouldHidePath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		lower := strings.ToLower(part)
		if ignoredNames[lower] || sensitiveNames[lower] {
			return true
		}
	}
	return false
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
