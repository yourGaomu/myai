package local

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainhistory "myai/core/domain/history"
	"myai/core/history"
	"myai/core/sandbox"
	tooldef "myai/core/tool/tool"
)

type ShellTool struct {
	sandbox   sandbox.Sandbox
	workspace string
}

type shellArgs struct {
	Command        string `json:"command"`
	WorkDir        string `json:"work_dir"`
	TimeoutMS      int    `json:"timeout_ms"`
	MaxOutputBytes int    `json:"max_output_bytes"`
}

func NewShellTool(sandbox sandbox.Sandbox) *ShellTool {
	return &ShellTool{sandbox: sandbox}
}

func NewShellToolWithWorkspace(workspace string, sandbox sandbox.Sandbox) *ShellTool {
	return &ShellTool{workspace: workspace, sandbox: sandbox}
}

func (t *ShellTool) Name() string {
	return "shell"
}

func (t *ShellTool) Description() string {
	return "Run a shell command inside the configured local sandbox workspace and return stdout, stderr, exit code, timeout, and truncation status."
}

func (t *ShellTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute.",
			},
			"work_dir": map[string]any{
				"type":        "string",
				"description": "Workspace-relative directory where the command should run. Defaults to workspace root.",
			},
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "Command timeout in milliseconds. Defaults to 30000 and is capped by the sandbox.",
			},
			"max_output_bytes": map[string]any{
				"type":        "integer",
				"description": "Maximum bytes captured for stdout and stderr. Defaults to the sandbox limit.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ShellTool) Permission() tooldef.Permission {
	return tooldef.PermissionExecute
}

func (t *ShellTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	if t.sandbox == nil {
		return "", errors.New("sandbox is nil")
	}

	input, err := normalizeShellArgs(args)
	if err != nil {
		return "", err
	}

	// Shell 可能修改任意数量文件，因此执行前后扫描 workspace 并归入同一个任务检查点。
	recorder, before, historyErr := t.snapshotBeforeShell(ctx)
	result, err := t.sandbox.Run(ctx, sandbox.RunRequest{
		Command:        input.Command,
		WorkDir:        input.WorkDir,
		Timeout:        time.Duration(input.TimeoutMS) * time.Millisecond,
		MaxOutputBytes: input.MaxOutputBytes,
	})
	if err != nil {
		return "", err
	}
	if recorder != nil && before != nil {
		if _, recordErr := recorder.RecordWorkspaceChanges(ctx, before, history.RecordCommand{
			Title:  "shell " + input.Command,
			Reason: "shell command",
		}); recordErr != nil && historyErr == nil {
			historyErr = recordErr
		}
	}
	if historyErr != nil {
		result.ErrorMessage = appendShellHistoryError(result.ErrorMessage, historyErr)
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (t *ShellTool) snapshotBeforeShell(ctx context.Context) (*history.TaskWorkspaceRecorder, map[string]domainhistory.FileSnapshot, error) {
	task := history.TaskRecorderFromContext(ctx)
	if task == nil {
		return nil, nil, nil
	}

	workspace, err := toolWorkspace(t.workspace)
	if err != nil {
		return nil, nil, err
	}
	recorder, err := task.WorkspaceRecorder(workspace)
	if err != nil {
		return nil, nil, err
	}
	before, err := recorder.SnapshotWorkspace(ctx)
	if err != nil {
		return nil, nil, err
	}
	return recorder, before, nil
}

func appendShellHistoryError(existing string, err error) string {
	if err == nil {
		return existing
	}
	text := "history error: " + err.Error()
	if strings.TrimSpace(existing) == "" {
		return text
	}
	return existing + "; " + text
}

func normalizeShellArgs(args json.RawMessage) (shellArgs, error) {
	var input shellArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return shellArgs{}, err
		}
	}

	input.Command = strings.TrimSpace(input.Command)
	input.WorkDir = strings.TrimSpace(input.WorkDir)
	if input.Command == "" {
		return shellArgs{}, errors.New("command is empty")
	}

	return input, nil
}
