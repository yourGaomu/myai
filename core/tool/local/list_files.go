package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tooldef "myai/core/tool/tool"
)

const (
	defaultListFilesLimit    = 200
	defaultListFilesMaxDepth = 1
	maxListFilesLimit        = 1000
	maxListFilesDepth        = 4
)

type ListFilesTool struct{}

type listFilesArgs struct {
	Path          string `json:"path"`
	Recursive     bool   `json:"recursive"`
	MaxDepth      int    `json:"max_depth"`
	Limit         int    `json:"limit"`
	IncludeHidden bool   `json:"include_hidden"`
}

type listFilesResult struct {
	Path      string          `json:"path"`
	Count     int             `json:"count"`
	Truncated bool            `json:"truncated"`
	Entries   []listFileEntry `json:"entries"`
}

type listFileEntry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Size  int64  `json:"size,omitempty"`
	Error string `json:"error,omitempty"`
}

func NewListFilesTool() *ListFilesTool {
	return &ListFilesTool{}
}

func (t *ListFilesTool) Name() string {
	return "list_files"
}

func (t *ListFilesTool) Description() string {
	return "List files and directories under a local workspace path. Use this before read_file when you need to discover project structure."
}

func (t *ListFilesTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to list. Defaults to current directory.",
			},
			"recursive": map[string]any{
				"type":        "boolean",
				"description": "Whether to recursively list child directories. Defaults to false.",
			},
			"max_depth": map[string]any{
				"type":        "integer",
				"description": "Maximum recursive depth. Defaults to 1 and is capped at 4.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of entries to return. Defaults to 200 and is capped at 1000.",
			},
			"include_hidden": map[string]any{
				"type":        "boolean",
				"description": "Whether to include hidden files and directories. Defaults to false.",
			},
		},
	}
}

func (t *ListFilesTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

func (t *ListFilesTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	input := normalizeListFilesArgs(args)
	entries := make([]listFileEntry, 0)
	truncated := false

	err := walkFiles(ctx, input.Path, input.Path, 1, input, &entries, &truncated)
	if err != nil {
		return "", err
	}

	result := listFilesResult{
		Path:      filepath.ToSlash(input.Path),
		Count:     len(entries),
		Truncated: truncated,
		Entries:   entries,
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func normalizeListFilesArgs(args json.RawMessage) listFilesArgs {
	input := listFilesArgs{
		Path:     ".",
		MaxDepth: defaultListFilesMaxDepth,
		Limit:    defaultListFilesLimit,
	}
	if len(args) > 0 {
		_ = json.Unmarshal(args, &input)
	}

	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		input.Path = "."
	}
	input.Path = filepath.Clean(input.Path)

	if input.Limit <= 0 {
		input.Limit = defaultListFilesLimit
	}
	if input.Limit > maxListFilesLimit {
		input.Limit = maxListFilesLimit
	}

	if input.MaxDepth <= 0 {
		input.MaxDepth = defaultListFilesMaxDepth
	}
	if input.MaxDepth > maxListFilesDepth {
		input.MaxDepth = maxListFilesDepth
	}
	if !input.Recursive {
		input.MaxDepth = 1
	}

	return input
}

func walkFiles(ctx context.Context, root string, dir string, depth int, input listFilesArgs, entries *[]listFileEntry, truncated *bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(*entries) >= input.Limit {
		*truncated = true
		return nil
	}

	children, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.Slice(children, func(i, j int) bool {
		leftIsDir := children[i].IsDir()
		rightIsDir := children[j].IsDir()
		if leftIsDir != rightIsDir {
			return leftIsDir
		}
		return strings.ToLower(children[i].Name()) < strings.ToLower(children[j].Name())
	})

	for _, child := range children {
		if len(*entries) >= input.Limit {
			*truncated = true
			return nil
		}
		if !input.IncludeHidden && strings.HasPrefix(child.Name(), ".") {
			continue
		}

		childPath := filepath.Join(dir, child.Name())
		entry := fileEntry(root, childPath, child)
		*entries = append(*entries, entry)

		if child.IsDir() && input.Recursive && depth < input.MaxDepth {
			if err := walkFiles(ctx, root, childPath, depth+1, input, entries, truncated); err != nil {
				*entries = append(*entries, listFileEntry{
					Path:  filepath.ToSlash(relativePath(root, childPath)),
					Name:  child.Name(),
					Type:  "dir",
					Error: err.Error(),
				})
			}
		}
	}

	return nil
}

func fileEntry(root string, path string, entry os.DirEntry) listFileEntry {
	item := listFileEntry{
		Path: filepath.ToSlash(relativePath(root, path)),
		Name: entry.Name(),
		Type: "file",
	}
	if entry.IsDir() {
		item.Type = "dir"
		return item
	}

	info, err := entry.Info()
	if err != nil {
		item.Error = err.Error()
		return item
	}
	item.Size = info.Size()
	return item
}

func relativePath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	if rel == "." {
		return filepath.Base(path)
	}
	return rel
}
