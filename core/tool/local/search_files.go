package local

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	tooldef "myai/core/tool/tool"
)

const (
	defaultSearchFilesLimit   = 100
	maxSearchFilesLimit       = 500
	defaultSearchMaxFileBytes = 1024 * 1024
)

var skippedSearchDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".idea":        true,
	".vscode":      true,
}

type SearchFilesTool struct {
	workspace string
}

type searchFilesArgs struct {
	Path          string `json:"path"`
	Query         string `json:"query"`
	Limit         int    `json:"limit"`
	IncludeHidden bool   `json:"include_hidden"`
	CaseSensitive bool   `json:"case_sensitive"`
}

type searchFilesResult struct {
	Path      string            `json:"path"`
	Query     string            `json:"query"`
	Count     int               `json:"count"`
	Truncated bool              `json:"truncated"`
	Matches   []searchFileMatch `json:"matches"`
}

type searchFileMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func NewSearchFilesTool() *SearchFilesTool {
	return &SearchFilesTool{}
}

func NewSearchFilesToolWithWorkspace(workspace string) *SearchFilesTool {
	return &SearchFilesTool{workspace: workspace}
}

func (t *SearchFilesTool) Name() string {
	return "search_files"
}

func (t *SearchFilesTool) Description() string {
	return "Search text files under a local workspace path by keyword and return matching file paths, line numbers, and snippets."
}

func (t *SearchFilesTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to search. Defaults to current directory.",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Text keyword to search for.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of matches to return. Defaults to 100 and is capped at 500.",
			},
			"include_hidden": map[string]any{
				"type":        "boolean",
				"description": "Whether to search hidden files and directories. Defaults to false.",
			},
			"case_sensitive": map[string]any{
				"type":        "boolean",
				"description": "Whether matching should be case-sensitive. Defaults to false.",
			},
		},
		"required": []string{"query"},
	}
}

func (t *SearchFilesTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

func (t *SearchFilesTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	workspace, err := toolWorkspace(t.workspace)
	if err != nil {
		return "", err
	}
	input, err := normalizeSearchFilesArgs(workspace, args)
	if err != nil {
		return "", err
	}

	matches := make([]searchFileMatch, 0)
	truncated := false
	err = filepath.WalkDir(input.Path, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return nil
		}
		if len(matches) >= input.Limit {
			truncated = true
			return filepath.SkipAll
		}
		if shouldSkipSearchEntry(path, entry, input.IncludeHidden) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		fileMatches, err := searchOneFile(input.Path, path, input)
		if err != nil {
			return nil
		}
		for _, match := range fileMatches {
			if len(matches) >= input.Limit {
				truncated = true
				return filepath.SkipAll
			}
			matches = append(matches, match)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Path != matches[j].Path {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].Line < matches[j].Line
	})

	result := searchFilesResult{
		Path:      filepath.ToSlash(relativePath(workspace, input.Path)),
		Query:     input.Query,
		Count:     len(matches),
		Truncated: truncated,
		Matches:   matches,
	}
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func normalizeSearchFilesArgs(workspace string, args json.RawMessage) (searchFilesArgs, error) {
	input := searchFilesArgs{
		Path:  ".",
		Limit: defaultSearchFilesLimit,
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return searchFilesArgs{}, err
		}
	}

	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		input.Path = "."
	}
	path, err := cleanWorkspacePath(workspace, input.Path)
	if err != nil {
		return searchFilesArgs{}, err
	}
	input.Path = path
	input.Query = strings.TrimSpace(input.Query)

	if input.Limit <= 0 {
		input.Limit = defaultSearchFilesLimit
	}
	if input.Limit > maxSearchFilesLimit {
		input.Limit = maxSearchFilesLimit
	}

	return input, nil
}

func shouldSkipSearchEntry(path string, entry os.DirEntry, includeHidden bool) bool {
	name := entry.Name()
	if !includeHidden && strings.HasPrefix(name, ".") {
		return true
	}
	if entry.IsDir() && skippedSearchDirs[name] {
		return true
	}
	return false
}

func searchOneFile(root string, path string, input searchFilesArgs) ([]searchFileMatch, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > defaultSearchMaxFileBytes {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	query := input.Query
	if !input.CaseSensitive {
		query = strings.ToLower(query)
	}

	matches := make([]searchFileMatch, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), defaultSearchMaxFileBytes)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !utf8.ValidString(line) {
			return nil, nil
		}

		searchLine := line
		if !input.CaseSensitive {
			searchLine = strings.ToLower(searchLine)
		}
		if strings.Contains(searchLine, query) {
			matches = append(matches, searchFileMatch{
				Path: filepath.ToSlash(relativePath(root, path)),
				Line: lineNumber,
				Text: strings.TrimSpace(line),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return matches, nil
}
