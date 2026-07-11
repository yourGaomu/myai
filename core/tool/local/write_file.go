package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myai/core/history"
	tooldef "myai/core/tool/tool"
)

type WriteFileTool struct {
	recorder  historyRecorder
	workspace string
}

type writeFileArgs struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Append     bool   `json:"append"`
	Overwrite  bool   `json:"overwrite"`
	CreateDirs bool   `json:"create_dirs"`
}

type writeFileResult struct {
	Path         string `json:"path"`
	Bytes        int    `json:"bytes"`
	Operation    string `json:"operation"`
	CheckpointID string `json:"checkpoint_id,omitempty"`
	HistoryError string `json:"history_error,omitempty"`
}

func NewWriteFileTool() *WriteFileTool {
	return &WriteFileTool{}
}

func NewWriteFileToolWithRecorder(recorder historyRecorder) *WriteFileTool {
	return &WriteFileTool{recorder: recorder}
}

func NewWriteFileToolWithWorkspace(workspace string) *WriteFileTool {
	return &WriteFileTool{workspace: workspace}
}

func NewWriteFileToolWithWorkspaceAndRecorder(workspace string, recorder historyRecorder) *WriteFileTool {
	return &WriteFileTool{workspace: workspace, recorder: recorder}
}

func (t *WriteFileTool) Permission() tooldef.Permission {
	return tooldef.PermissionWrite
}

func (t *WriteFileTool) Description() string {
	return "Write or append UTF-8 text content to a local workspace file. Use overwrite=true when replacing an existing file."
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path to write.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "UTF-8 text content to write to the file.",
			},
			"append": map[string]any{
				"type":        "boolean",
				"description": "Append to the file instead of replacing it. Defaults to false.",
			},
			"overwrite": map[string]any{
				"type":        "boolean",
				"description": "Allow replacing an existing file when append=false. Defaults to false.",
			},
			"create_dirs": map[string]any{
				"type":        "boolean",
				"description": "Create parent directories when they do not exist. Defaults to true.",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	workspace, err := toolWorkspace(t.workspace)
	if err != nil {
		return "", err
	}

	input, err := normalizeWriteFileArgs(workspace, args)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	recorder, closeRecorder, err := openHistoryRecorder(ctx, t.recorder, workspace)
	if err != nil {
		return "", err
	}
	defer closeRecorder()

	// 与 edit_file 使用相同历史协议：先取 before，写入成功后再记录最终文件状态。
	before, err := recorder.SnapshotPath(input.Path)
	if err != nil {
		return "", err
	}

	parent := filepath.Dir(input.Path)
	if input.CreateDirs && parent != "." {
		if err := os.MkdirAll(parent, 0755); err != nil {
			return "", err
		}
	}

	if info, err := os.Stat(input.Path); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("path is a directory: %s", input.Path)
		}
		if !input.Append && !input.Overwrite {
			return "", fmt.Errorf("file already exists: %s; set overwrite=true to replace it", input.Path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	operation := "write"
	if input.Append {
		operation = "append"
		err = appendFile(input.Path, input.Content)
	} else {
		err = os.WriteFile(input.Path, []byte(input.Content), 0644)
	}
	if err != nil {
		return "", err
	}

	result := writeFileResult{
		Path:      filepath.ToSlash(relativePath(workspace, input.Path)),
		Bytes:     len([]byte(input.Content)),
		Operation: operation,
	}
	checkpointID, err := recorder.RecordFileChange(ctx, input.Path, before, history.RecordCommand{
		Title:  "write_file " + result.Path,
		Reason: operation,
	})
	if err != nil {
		result.HistoryError = err.Error()
	} else {
		result.CheckpointID = checkpointID
	}
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func normalizeWriteFileArgs(workspace string, args json.RawMessage) (writeFileArgs, error) {
	input := writeFileArgs{
		CreateDirs: true,
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return writeFileArgs{}, err
		}
	}

	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		return writeFileArgs{}, errors.New("path is empty")
	}
	path, err := cleanWorkspaceWritePath(workspace, input.Path)
	if err != nil {
		return writeFileArgs{}, err
	}
	input.Path = path

	return input, nil
}

func cleanWorkspaceWritePath(workspace string, path string) (string, error) {
	return cleanWorkspacePath(workspace, path)
}

func cleanWorkspacePath(workspace string, path string) (string, error) {
	workspace, err := toolWorkspace(workspace)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(workspace, absPath)
	if err != nil {
		return "", fmt.Errorf("path is outside workspace: %s", path)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside workspace: %s", path)
	}

	return absPath, nil
}

func toolWorkspace(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		current, err := os.Getwd()
		if err != nil {
			return "", err
		}
		workspace = current
	}
	return filepath.Abs(filepath.Clean(workspace))
}

func appendFile(path string, content string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}
