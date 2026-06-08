package files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"myai/core/remote/protocol"
)

const (
	defaultListLimit = 200
	maxListLimit     = 1000
	maxReadBytes     = 256 * 1024
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

type Service struct {
	root string
}

func (s *Service) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func New(root string) (*Service, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory: %s", abs)
	}
	return &Service{root: abs}, nil
}

func MustNew(root string) *Service {
	service, err := New(root)
	if err != nil {
		return &Service{root: "."}
	}
	return service
}

func (s *Service) List(ctx context.Context, payload protocol.FileListPayload) (protocol.FileListResultPayload, error) {
	if err := ctx.Err(); err != nil {
		return protocol.FileListResultPayload{}, err
	}

	limit := payload.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	abs, rel, err := s.resolve(payload.Path)
	if err != nil {
		return protocol.FileListResultPayload{}, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return protocol.FileListResultPayload{}, err
	}
	if !info.IsDir() {
		return protocol.FileListResultPayload{}, fmt.Errorf("path is not a directory: %s", rel)
	}

	children, err := os.ReadDir(abs)
	if err != nil {
		return protocol.FileListResultPayload{}, err
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].IsDir() != children[j].IsDir() {
			return children[i].IsDir()
		}
		return strings.ToLower(children[i].Name()) < strings.ToLower(children[j].Name())
	})

	entries := make([]protocol.FileEntry, 0, minInt(len(children), limit))
	truncated := false
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return protocol.FileListResultPayload{}, err
		}
		if len(entries) >= limit {
			truncated = true
			break
		}
		if shouldHide(child.Name(), payload.IncludeHidden) {
			continue
		}

		childAbs := filepath.Join(abs, child.Name())
		childRel, err := s.relative(childAbs)
		if err != nil {
			continue
		}
		entry := protocol.FileEntry{
			Path: childRel,
			Name: child.Name(),
			Type: "file",
		}
		if child.IsDir() {
			entry.Type = "dir"
		}
		if info, err := child.Info(); err == nil {
			entry.Size = info.Size()
			entry.Modified = info.ModTime()
		}
		entries = append(entries, entry)
	}

	return protocol.FileListResultPayload{
		Path:      rel,
		Parent:    parentPath(rel),
		Entries:   entries,
		Count:     len(entries),
		Truncated: truncated,
	}, nil
}

func (s *Service) Read(ctx context.Context, payload protocol.FileReadPayload) (protocol.FileReadResultPayload, error) {
	if err := ctx.Err(); err != nil {
		return protocol.FileReadResultPayload{}, err
	}
	if strings.TrimSpace(payload.Path) == "" {
		return protocol.FileReadResultPayload{}, errors.New("file path is empty")
	}
	if sensitiveNames[strings.ToLower(filepath.Base(payload.Path))] {
		return protocol.FileReadResultPayload{}, fmt.Errorf("refusing to preview sensitive file: %s", payload.Path)
	}

	abs, rel, err := s.resolve(payload.Path)
	if err != nil {
		return protocol.FileReadResultPayload{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return protocol.FileReadResultPayload{}, err
	}
	if info.IsDir() {
		return protocol.FileReadResultPayload{}, fmt.Errorf("path is a directory: %s", rel)
	}

	file, err := os.Open(abs)
	if err != nil {
		return protocol.FileReadResultPayload{}, err
	}
	defer file.Close()

	readLimit := maxReadBytes
	if info.Size() < int64(readLimit) {
		readLimit = int(info.Size())
	}
	buffer := make([]byte, readLimit)
	n, err := io.ReadFull(file, buffer)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return protocol.FileReadResultPayload{}, err
	}
	content := buffer[:n]
	binary := isBinary(content)
	result := protocol.FileReadResultPayload{
		Path:      rel,
		Name:      filepath.Base(rel),
		Language:  languageFromPath(rel),
		Size:      info.Size(),
		Truncated: info.Size() > int64(maxReadBytes),
		Binary:    binary,
	}
	if !binary {
		result.Content = string(content)
	}
	return result, nil
}

func (s *Service) resolve(path string) (string, string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		clean = "."
	}
	if filepath.IsAbs(clean) {
		return "", "", errors.New("absolute paths are not allowed")
	}

	abs := filepath.Join(s.root, clean)
	abs = filepath.Clean(abs)
	if target, err := filepath.EvalSymlinks(abs); err == nil {
		abs = target
	}
	if !insideRoot(s.root, abs) {
		return "", "", fmt.Errorf("path escapes workspace: %s", path)
	}

	rel, err := s.relative(abs)
	if err != nil {
		return "", "", err
	}
	return abs, rel, nil
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

func shouldHide(name string, includeHidden bool) bool {
	lower := strings.ToLower(name)
	if sensitiveNames[lower] || ignoredNames[lower] {
		return true
	}
	if strings.HasPrefix(name, ".") && !includeHidden {
		return true
	}
	return false
}

func parentPath(path string) string {
	if path == "" || path == "." {
		return ""
	}
	parent := filepath.ToSlash(filepath.Dir(path))
	if parent == "." {
		return "."
	}
	return parent
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

func languageFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".json":
		return "json"
	case ".md", ".markdown":
		return "markdown"
	case ".css":
		return "css"
	case ".html":
		return "html"
	case ".yaml", ".yml":
		return "yaml"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".sh", ".ps1", ".bat":
		return "shell"
	default:
		return "text"
	}
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
