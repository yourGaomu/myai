package local

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	localexecutor "myai/core/adapter/execution/local"
	domainexecution "myai/core/domain/execution"
	domainhistory "myai/core/domain/history"
	domainsandbox "myai/core/domain/sandbox"
	domaintool "myai/core/domain/tool"
	"myai/core/history"
	executionport "myai/core/port/execution"
	workspaceport "myai/core/port/workspace"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type ShellTool struct {
	local     executionport.CommandExecutor
	remote    workspaceport.CommandRunner
	workspace string
}

type shellArgs struct {
	Command        string `json:"command"`
	WorkDir        string `json:"work_dir"`
	TimeoutMS      int    `json:"timeout_ms"`
	MaxOutputBytes int    `json:"max_output_bytes"`
}

type shellResultPayload struct {
	Command              string `json:"command"`
	WorkDir              string `json:"work_dir"`
	ExitCode             int    `json:"exit_code"`
	Stdout               string `json:"stdout"`
	Stderr               string `json:"stderr"`
	TimedOut             bool   `json:"timed_out"`
	Truncated            bool   `json:"truncated"`
	DurationMS           int64  `json:"duration_ms"`
	ExecutionEnvironment string `json:"execution_environment"`
	Isolated             bool   `json:"isolated"`
	Shell                string `json:"shell"`
	ErrorMessage         string `json:"error,omitempty"`
}

func NewShellToolWithWorkspace(workspace string, local executionport.CommandExecutor) *ShellTool {
	return &ShellTool{workspace: workspace, local: local}
}

func NewShellToolWithWorkspaceAndRemote(workspace string, local executionport.CommandExecutor, remote workspaceport.CommandRunner) *ShellTool {
	return &ShellTool{workspace: workspace, local: local, remote: remote}
}

func (t *ShellTool) Name() string {
	return "shell"
}

func (t *ShellTool) Description() string {
	return "Run a shell command in the session execution environment. Local-host execution is not OS-isolated; OpenSandbox sessions are isolated."
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
				"description": "Command timeout in milliseconds. Defaults to 30000 and is capped by the executor.",
			},
			"max_output_bytes": map[string]any{
				"type":        "integer",
				"description": "Maximum bytes captured for stdout and stderr. Defaults to the executor limit.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ShellTool) Permission() tooldef.Permission {
	return tooldef.PermissionExecute
}

func (t *ShellTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	execution, _ := toolruntime.CurrentExecution(ctx)
	if t.local == nil && (execution.SandboxID == "" || t.remote == nil) {
		return tooldef.ToolOutput{}, errors.New("command executor is nil")
	}

	input, err := normalizeShellArgs(args)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}

	// Shell 可能修改任意数量文件，因此执行前后扫描 workspace 并归入同一个任务检查点。
	recorder, before, historyErr := t.snapshotBeforeShell(ctx)
	request := domainexecution.RunRequest{
		Command: input.Command, WorkDir: input.WorkDir,
		Timeout: time.Duration(input.TimeoutMS) * time.Millisecond, MaxOutputBytes: input.MaxOutputBytes,
	}
	result, err := t.run(ctx, execution, request)
	if err != nil {
		return tooldef.ToolOutput{}, err
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

	output, err := json.MarshalIndent(shellPayload(result), "", "  ")
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	resultOutput := tooldef.SuccessOutput(string(output))
	resultOutput.Truncated = result.Truncated
	if result.TimedOut {
		resultOutput.Status = domaintool.ResultStatusTimeout
		resultOutput.ErrorCode = "command_timeout"
		resultOutput.ErrorMessage = result.ErrorMessage
	} else if result.ExitCode != 0 {
		resultOutput.Status = domaintool.ResultStatusFailed
		resultOutput.ErrorCode = "command_exit_nonzero"
		resultOutput.ErrorMessage = result.ErrorMessage
	}
	return resultOutput, nil
}

func shellPayload(result domainexecution.RunResult) shellResultPayload {
	return shellResultPayload{
		Command: result.Command, WorkDir: result.WorkDir, ExitCode: result.ExitCode,
		Stdout: result.Stdout, Stderr: result.Stderr, TimedOut: result.TimedOut,
		Truncated: result.Truncated, DurationMS: result.DurationMS,
		ExecutionEnvironment: result.ExecutionEnvironment, Isolated: result.Isolated,
		Shell: result.Shell, ErrorMessage: result.ErrorMessage,
	}
}

func (t *ShellTool) run(ctx context.Context, execution toolruntime.Execution, request domainexecution.RunRequest) (domainexecution.RunResult, error) {
	if execution.SandboxID != "" {
		if t.remote == nil {
			return domainexecution.RunResult{}, errors.New("remote workspace command runner is not configured")
		}
		started := time.Now()
		result, err := t.remote.Run(ctx, execution.SandboxID, execution.WorkspaceRoot, domainsandbox.CommandRequest{
			Command: request.Command, WorkDir: request.WorkDir, Timeout: request.Timeout,
		})
		return domainexecution.RunResult{
			Command: request.Command, WorkDir: request.WorkDir, ExitCode: result.ExitCode,
			Stdout: result.Stdout, Stderr: result.Stderr, TimedOut: result.TimedOut,
			DurationMS: time.Since(started).Milliseconds(), ExecutionEnvironment: "opensandbox", Isolated: true, Shell: "remote",
			ErrorMessage: joinRemoteError(result.ErrorName, result.ErrorValue),
		}, err
	}
	runner := t.local
	if execution.WorkspaceRoot != "" {
		var err error
		runner, err = localexecutor.New(execution.WorkspaceRoot)
		if err != nil {
			return domainexecution.RunResult{}, err
		}
	}
	return runner.Run(ctx, request)
}

func joinRemoteError(name string, value string) string {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	switch {
	case name == "":
		return value
	case value == "":
		return name
	default:
		return name + ": " + value
	}
}

func (t *ShellTool) snapshotBeforeShell(ctx context.Context) (*history.TaskWorkspaceRecorder, map[string]domainhistory.FileSnapshot, error) {
	task := history.TaskRecorderFromContext(ctx)
	if task == nil {
		return nil, nil, nil
	}

	workspace, err := toolWorkspace(toolruntime.WorkspaceRoot(ctx, t.workspace))
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
