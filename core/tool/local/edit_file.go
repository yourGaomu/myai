package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"myai/core/history"
	tooldef "myai/core/tool/tool"
)

type EditFileTool struct {
	recorder  historyRecorder
	workspace string
}

type editFileArgs struct {
	Path       string `json:"path"`
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll bool   `json:"replace_all"`
}

type editFileResult struct {
	Path         string `json:"path"`
	Replacements int    `json:"replacements"`
	Bytes        int    `json:"bytes"`
	CheckpointID string `json:"checkpoint_id,omitempty"`
	HistoryError string `json:"history_error,omitempty"`
}

func NewEditFileTool() *EditFileTool {
	return &EditFileTool{}
}

func NewEditFileToolWithRecorder(recorder historyRecorder) *EditFileTool {
	return &EditFileTool{recorder: recorder}
}

func NewEditFileToolWithWorkspace(workspace string) *EditFileTool {
	return &EditFileTool{workspace: workspace}
}

func NewEditFileToolWithWorkspaceAndRecorder(workspace string, recorder historyRecorder) *EditFileTool {
	return &EditFileTool{workspace: workspace, recorder: recorder}
}

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Description() string {
	return "Edit a local workspace text file by replacing an exact old_text snippet with new_text. Use replace_all=true only when every occurrence should be replaced."
}

func (t *EditFileTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path to edit.",
			},
			"old_text": map[string]any{
				"type":        "string",
				"description": "Exact text currently in the file.",
			},
			"new_text": map[string]any{
				"type":        "string",
				"description": "Replacement text.",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace every occurrence instead of requiring exactly one match. Defaults to false.",
			},
		},
		"required": []string{"path", "old_text", "new_text"},
	}
}

func (t *EditFileTool) Permission() tooldef.Permission {
	return tooldef.PermissionWrite
}

func (t *EditFileTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	workspace, err := toolWorkspace(t.workspace)
	if err != nil {
		return "", err
	}

	input, err := normalizeEditFileArgs(workspace, args)
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

	// 写入前先保存快照；成功后记录 before/after，手机才能按检查点恢复。
	before, err := recorder.SnapshotPath(input.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(input.Path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory: %s", input.Path)
	}

	contentBytes, err := os.ReadFile(input.Path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(contentBytes) {
		return "", fmt.Errorf("file is not valid UTF-8 text: %s", input.Path)
	}

	content := string(contentBytes)
	replacements := strings.Count(content, input.OldText)
	if replacements == 0 {
		return "", errors.New("old_text was not found in file")
	}
	if replacements > 1 && !input.ReplaceAll {
		return "", fmt.Errorf("old_text appears %d times; set replace_all=true to replace every occurrence", replacements)
	}

	nextContent := strings.Replace(content, input.OldText, input.NewText, replacementLimit(input.ReplaceAll))
	if err := os.WriteFile(input.Path, []byte(nextContent), info.Mode().Perm()); err != nil {
		return "", err
	}

	result := editFileResult{
		Path:         filepath.ToSlash(relativePath(workspace, input.Path)),
		Replacements: replacementCount(replacements, input.ReplaceAll),
		Bytes:        len([]byte(nextContent)),
	}
	checkpointID, err := recorder.RecordFileChange(ctx, input.Path, before, history.RecordCommand{
		Title:  "edit_file " + result.Path,
		Reason: "replace text",
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

func normalizeEditFileArgs(workspace string, args json.RawMessage) (editFileArgs, error) {
	var input editFileArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return editFileArgs{}, err
		}
	}

	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		return editFileArgs{}, errors.New("path is empty")
	}
	if input.OldText == "" {
		return editFileArgs{}, errors.New("old_text is empty")
	}

	path, err := cleanWorkspaceWritePath(workspace, input.Path)
	if err != nil {
		return editFileArgs{}, err
	}
	input.Path = path

	return input, nil
}

func replacementLimit(replaceAll bool) int {
	if replaceAll {
		return -1
	}
	return 1
}

func replacementCount(total int, replaceAll bool) int {
	if replaceAll {
		return total
	}
	return 1
}
