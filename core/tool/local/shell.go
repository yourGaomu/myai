package local

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"myai/core/sandbox"
	tooldef "myai/core/tool/tool"
)

type ShellTool struct {
	sandbox sandbox.Sandbox
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

	result, err := t.sandbox.Run(ctx, sandbox.RunRequest{
		Command:        input.Command,
		WorkDir:        input.WorkDir,
		Timeout:        time.Duration(input.TimeoutMS) * time.Millisecond,
		MaxOutputBytes: input.MaxOutputBytes,
	})
	if err != nil {
		return "", err
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
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
