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
	result := toolresult.Execution{Messages: make([]domainmessage.Message, 0, len(command.Calls)), Entries: make([]domaintool.ExecutionEntry, 0, len(command.Calls)*2), Assets: make([]domaintool.SharedAsset, 0)}
	createdAt := s.now()
	for index, call := range command.Calls {
		if strings.TrimSpace(call.Name) == "" {
			continue
		}
		registeredTool, err := s.Registry.GetTool(call.Name)
		if err != nil {
			return toolresult.Execution{}, err
		}
		permission := tooldef.NormalizePermission(registeredTool.Permission())
		// PreToolUse 可拒绝调用或重写参数；显式 Allow 才能跳过默认权限询问。
		hookResult, err := s.beforeToolUse(ctx, toolcommand.HookEvent{SessionID: command.SessionID, Name: call.Name, Arguments: call.Arguments, Permission: permission})
		if err != nil {
			return toolresult.Execution{}, err
		}
		if strings.TrimSpace(hookResult.Arguments) != "" {
			call.Arguments = hookResult.Arguments
		}
		if command.Callbacks.OnToolCall != nil {
			command.Callbacks.OnToolCall(call.Name, call.Arguments)
		}
		callCreatedAt := createdAt.Add(time.Duration(index*2) * time.Nanosecond)
		resultCreatedAt := createdAt.Add(time.Duration(index*2+1) * time.Nanosecond)
		result.Entries = append(result.Entries, ToolCallEntry(toolcommand.ToolCallEntry{SessionID: command.SessionID, Call: call, CreatedAt: callCreatedAt}))

		if hookResult.Decision == toolresult.HookDecisionDeny {
			err = fmt.Errorf("tool denied by hook: %s", hookResult.Message)
			toolOutput := "tool error: " + err.Error()
			if command.Callbacks.OnToolResult != nil {
				command.Callbacks.OnToolResult(call.Name, call.Arguments, toolOutput)
			}
			s.afterToolUse(ctx, toolcommand.HookEvent{SessionID: command.SessionID, Name: call.Name, Arguments: call.Arguments, Permission: permission, Result: toolOutput, Err: err})
			result.Messages = append(result.Messages, ToolResultMessage(call, toolOutput))
			result.Entries = append(result.Entries, ToolResultEntry(toolcommand.ToolResultEntry{SessionID: command.SessionID, Call: call, Result: toolOutput, ToolError: err.Error(), CreatedAt: resultCreatedAt}))
			continue
		}

		permissionDecision := s.permissionService().Allow(toolcommand.Permission{Name: call.Name, Arguments: call.Arguments, Permission: permission, Mode: command.PermissionMode, HookAllowed: hookResult.Decision == toolresult.HookDecisionAllow, Ask: command.Callbacks.OnToolAsk})
		toolOutput := permissionDecision.Message
		var toolErr error
		if permissionDecision.Allowed {
			toolOutput, toolErr = registeredTool.Call(ctx, []byte(call.Arguments))
		}
		if toolErr != nil {
			toolOutput = "tool error: " + toolErr.Error()
		}
		if command.Callbacks.OnToolResult != nil {
			command.Callbacks.OnToolResult(call.Name, call.Arguments, toolOutput)
		}
		s.afterToolUse(ctx, toolcommand.HookEvent{SessionID: command.SessionID, Name: call.Name, Arguments: call.Arguments, Permission: permission, Result: toolOutput, Err: toolErr})
		toolError := ""
		if toolErr != nil {
			toolError = toolErr.Error()
		}
		// 成功结果可能包含上传文件信息，提取后作为共享资源单独持久化并展示给手机。
		if toolError == "" && s.Assets != nil {
			if asset, ok := s.Assets.Extract(toolcommand.AssetExtraction{SessionID: command.SessionID, RequestID: command.RequestID, Call: call, Result: toolOutput, CreatedAt: resultCreatedAt}); ok {
				result.Assets = append(result.Assets, asset)
			}
		}
		result.Messages = append(result.Messages, ToolResultMessage(call, toolOutput))
		result.Entries = append(result.Entries, ToolResultEntry(toolcommand.ToolResultEntry{SessionID: command.SessionID, Call: call, Result: toolOutput, ToolError: toolError, CreatedAt: resultCreatedAt}))
	}
	return result, nil
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
