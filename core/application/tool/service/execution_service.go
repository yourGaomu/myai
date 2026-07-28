package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	toolapi "myai/core/application/tool/api"
	toolcommand "myai/core/application/tool/command"
	toolport "myai/core/application/tool/port"
	toolresult "myai/core/application/tool/result"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

type ExecutionService struct {
	// 每个工具调用按 Hook -> 权限 -> 执行 -> 结果 Hook -> Asset 提取的顺序处理。
	Registry    toolport.Registry
	Hooks       toolport.HookBridge
	Permissions toolapi.PermissionService
	Assets      toolport.AssetExtractor
	Now         func() time.Time
}

var _ toolapi.ExecutionService = ExecutionService{}

func (s ExecutionService) Execute(ctx context.Context, command toolcommand.Execution) (toolresult.Execution, error) {
	if s.Registry == nil {
		return toolresult.Execution{}, errors.New("tool registry is nil")
	}
	result := toolresult.Execution{Calls: make([]domainmessage.ToolCall, 0, len(command.Calls)), Messages: make([]domainmessage.Message, 0, len(command.Calls)), Entries: make([]domaintool.ExecutionEntry, 0, len(command.Calls)*2), Assets: make([]domaintool.SharedAsset, 0)}
	createdAt := s.now()
	for index, call := range command.Calls {
		if strings.TrimSpace(call.Name) == "" {
			continue
		}
		registeredTool, err := s.Registry.GetTool(call.Name)
		if err != nil {
			return s.failBeforeExecution(result, command, call, index, "tool_not_found", err)
		}
		permission := tooldef.NormalizePermission(registeredTool.Permission())
		// PreToolUse 可拒绝调用或重写参数；Allow 只表示 Hook 放行，不能绕过权限检查。
		hookResult, err := s.beforeToolUse(ctx, toolcommand.HookEvent{SessionID: command.SessionID, Name: call.Name, Arguments: call.Arguments, Permission: permission})
		if err != nil {
			return s.failBeforeExecution(result, command, call, index, "hook_pre_failed", err)
		}
		if strings.TrimSpace(hookResult.Arguments) != "" {
			call.Arguments = hookResult.Arguments
		}
		result.Calls = append(result.Calls, call)
		if command.Callbacks.OnToolCall != nil {
			command.Callbacks.OnToolCall(call.Name, call.Arguments)
		}
		callCreatedAt := createdAt.Add(time.Duration(index*2) * time.Nanosecond)
		resultCreatedAt := createdAt.Add(time.Duration(index*2+1) * time.Nanosecond)
		result.Entries = append(result.Entries, ToolCallEntry(toolcommand.ToolCallEntry{SessionID: command.SessionID, Call: call, CreatedAt: callCreatedAt}))
		if !toolAllowed(command.AllowedTools, command.EnforceToolAllowlist, call.Name) {
			message := fmt.Sprintf("tool %s is not allowed for this subagent", call.Name)
			output := domaintool.FailedOutput(domaintool.ResultStatusDenied, "subagent_tool_denied", message)
			if command.Callbacks.OnToolResult != nil {
				command.Callbacks.OnToolResult(call.Name, call.Arguments, output)
			}
			s.afterToolUse(ctx, toolcommand.HookEvent{SessionID: command.SessionID, Name: call.Name, Arguments: call.Arguments, Permission: permission, Result: output.Content, Err: outputError(output, nil)})
			result.Messages = append(result.Messages, ToolResultMessage(call, output))
			result.Entries = append(result.Entries, ToolResultEntry(toolcommand.ToolResultEntry{SessionID: command.SessionID, Call: call, Output: output, CreatedAt: resultCreatedAt}))
			continue
		}

		if hookResult.Decision == toolresult.HookDecisionDeny {
			err = fmt.Errorf("tool denied by hook: %s", hookResult.Message)
			output := domaintool.FailedOutput(domaintool.ResultStatusDenied, "hook_denied", "tool error: "+err.Error())
			if command.Callbacks.OnToolResult != nil {
				command.Callbacks.OnToolResult(call.Name, call.Arguments, output)
			}
			s.afterToolUse(ctx, toolcommand.HookEvent{SessionID: command.SessionID, Name: call.Name, Arguments: call.Arguments, Permission: permission, Result: output.Content, Err: err})
			result.Messages = append(result.Messages, ToolResultMessage(call, output))
			result.Entries = append(result.Entries, ToolResultEntry(toolcommand.ToolResultEntry{SessionID: command.SessionID, Call: call, Output: output, CreatedAt: resultCreatedAt}))
			continue
		}
		if !allowsExecutionInMode(permission, command.AgentMode, command.ForceChatMode) {
			message := fmt.Sprintf("permission denied: plan mode does not allow tool %s requiring %s", call.Name, permission)
			output := domaintool.FailedOutput(domaintool.ResultStatusDenied, "plan_mode_denied", message)
			if command.Callbacks.OnToolResult != nil {
				command.Callbacks.OnToolResult(call.Name, call.Arguments, output)
			}
			s.afterToolUse(ctx, toolcommand.HookEvent{SessionID: command.SessionID, Name: call.Name, Arguments: call.Arguments, Permission: permission, Result: output.Content, Err: outputError(output, nil)})
			result.Messages = append(result.Messages, ToolResultMessage(call, output))
			result.Entries = append(result.Entries, ToolResultEntry(toolcommand.ToolResultEntry{SessionID: command.SessionID, Call: call, Output: output, CreatedAt: resultCreatedAt}))
			continue
		}

		permissionDecision := s.permissionService().Allow(toolcommand.Permission{
			Name: call.Name, Arguments: call.Arguments, Permission: permission, Mode: command.PermissionMode,
			RequireConfirmation: hookResult.Decision == toolresult.HookDecisionAsk,
			Ask:                 command.Callbacks.OnToolAsk,
		})
		output := domaintool.ToolOutput{}
		var toolErr error
		if permissionDecision.Allowed {
			output, toolErr = registeredTool.Call(ctx, []byte(call.Arguments))
		} else {
			output = domaintool.FailedOutput(domaintool.ResultStatusDenied, "permission_denied", permissionDecision.Message)
		}
		if toolErr != nil {
			output = outputForError(ctx, toolErr)
		}
		output = output.Normalized()
		if command.Callbacks.OnToolResult != nil {
			command.Callbacks.OnToolResult(call.Name, call.Arguments, output)
		}
		s.afterToolUse(ctx, toolcommand.HookEvent{SessionID: command.SessionID, Name: call.Name, Arguments: call.Arguments, Permission: permission, Result: output.Content, Err: outputError(output, toolErr)})
		// 成功结果可能包含上传文件信息，提取后作为共享资源单独持久化并展示给手机。
		if !output.Failed() && s.Assets != nil {
			if asset, ok := s.Assets.Extract(toolcommand.AssetExtraction{SessionID: command.SessionID, RequestID: command.RequestID, Call: call, Output: output, CreatedAt: resultCreatedAt}); ok {
				result.Assets = append(result.Assets, asset)
			}
		}
		result.Messages = append(result.Messages, ToolResultMessage(call, output))
		result.Entries = append(result.Entries, ToolResultEntry(toolcommand.ToolResultEntry{SessionID: command.SessionID, Call: call, Output: output, CreatedAt: resultCreatedAt}))
	}
	return result, nil
}

func (s ExecutionService) failBeforeExecution(result toolresult.Execution, command toolcommand.Execution, call domainmessage.ToolCall, index int, code string, err error) (toolresult.Execution, error) {
	createdAt := s.now()
	callCreatedAt := createdAt.Add(time.Duration(index*2) * time.Nanosecond)
	resultCreatedAt := createdAt.Add(time.Duration(index*2+1) * time.Nanosecond)
	output := domaintool.FailedOutput(domaintool.ResultStatusFailed, code, "tool error: "+err.Error())
	result.Calls = append(result.Calls, call)
	if command.Callbacks.OnToolCall != nil {
		command.Callbacks.OnToolCall(call.Name, call.Arguments)
	}
	if command.Callbacks.OnToolResult != nil {
		command.Callbacks.OnToolResult(call.Name, call.Arguments, output)
	}
	result.Messages = append(result.Messages, ToolResultMessage(call, output))
	result.Entries = append(result.Entries,
		ToolCallEntry(toolcommand.ToolCallEntry{SessionID: command.SessionID, Call: call, CreatedAt: callCreatedAt}),
		ToolResultEntry(toolcommand.ToolResultEntry{SessionID: command.SessionID, Call: call, Output: output, CreatedAt: resultCreatedAt}),
	)
	return result, err
}

func toolAllowed(allowed []string, enforced bool, name string) bool {
	if !enforced && allowed == nil {
		return true
	}
	for _, candidate := range allowed {
		if candidate == name {
			return true
		}
	}
	return false
}

func allowsExecutionInMode(permission tooldef.Permission, agentMode session.AgentMode, forceChatMode bool) bool {
	if forceChatMode || session.NormalizeAgentMode(agentMode) != session.AgentModePlan {
		return true
	}
	return tooldef.NormalizePermission(permission) == tooldef.PermissionRead
}

func outputForError(ctx context.Context, err error) domaintool.ToolOutput {
	status := domaintool.ResultStatusFailed
	code := "tool_failed"
	if errors.Is(err, context.DeadlineExceeded) {
		status = domaintool.ResultStatusTimeout
		code = "tool_timeout"
	} else if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		status = domaintool.ResultStatusCanceled
		code = "tool_canceled"
	}
	return domaintool.FailedOutput(status, code, "tool error: "+err.Error())
}

func outputError(output domaintool.ToolOutput, err error) error {
	if err != nil {
		return err
	}
	output = output.Normalized()
	if !output.Failed() {
		return nil
	}
	message := strings.TrimSpace(output.ErrorMessage)
	if message == "" {
		message = strings.TrimSpace(output.Content)
	}
	if message == "" {
		message = string(output.Status)
	}
	return errors.New(message)
}

func (s ExecutionService) beforeToolUse(ctx context.Context, event toolcommand.HookEvent) (toolresult.Hook, error) {
	if s.Hooks == nil {
		return toolresult.Hook{Decision: toolresult.HookDecisionContinue}, nil
	}
	return s.Hooks.BeforeToolUse(ctx, event)
}

func (s ExecutionService) afterToolUse(ctx context.Context, event toolcommand.HookEvent) {
	if s.Hooks != nil {
		s.Hooks.AfterToolUse(ctx, event)
	}
}

func (s ExecutionService) permissionService() toolapi.PermissionService {
	if s.Permissions != nil {
		return s.Permissions
	}
	return PermissionService{}
}

func (s ExecutionService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
