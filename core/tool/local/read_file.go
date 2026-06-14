package local

import (
	"context"
	"encoding/json"
	"os"

	tooldef "myai/core/tool/tool"
)

type ReadFileTool struct {
	workspace string
}

type readFileArgs struct {
	Path string `json:"path"`
}

func NewReadFileTool() *ReadFileTool {
	return &ReadFileTool{}
}

func NewReadFileToolWithWorkspace(workspace string) *ReadFileTool {
	return &ReadFileTool{workspace: workspace}
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read a UTF-8 text file from the local workspace."
}

func (t *ReadFileTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path to read.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

func (t *ReadFileTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var input readFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	path, err := cleanWorkspacePath(t.workspace, input.Path)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}
