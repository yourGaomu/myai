package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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

type callExecution struct {
	skip     bool
	call     domainmessage.ToolCall
	message  domainmessage.Message
	entries  []domaintool.ExecutionEntry
	asset    domaintool.SharedAsset
	hasAsset bool
}

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
	createdAt := s.now()
	slots := make([]callExecution, len(command.Calls))
	var wg sync.WaitGroup
	var exclusive sync.RWMutex
	for index, call := range command.Calls {
		if strings.TrimSpace(call.Name) == "" {
			slots[index].skip = true
			continue
		}
		wg.Add(1)
		go func(index int, call domainmessage.ToolCall) {
			defer wg.Done()
			permission := s.lookupPermission(call.Name)
			if permission == tooldef.PermissionRead {
				exclusive.RLock()
				defer exclusive.RUnlock()
			} else {
				exclusive.Lock()
				defer exclusive.Unlock()
			}
			slots[index] = s.executeOne(ctx, command, call, index, createdAt)
		}(index, call)
	}
	wg.Wait()

	result := toolresult.Execution{
		Calls:    make([]domainmessage.ToolCall, 0, len(command.Calls)),
		Messages: make([]domainmessage.Message, 0, len(command.Calls)),
		Entries:  make([]domaintool.ExecutionEntry, 0, len(command.Calls)*2),
		Assets:   make([]domaintool.SharedAsset, 0),
	}
	for _, slot := range slots {
		if slot.skip {
			continue
		}
		result.Calls = append(result.Calls, slot.call)
		result.Messages = append(result.Messages, slot.message)
		result.Entries = append(result.Entries, slot.entries...)
		if slot.hasAsset {
			result.Assets = append(result.Assets, slot.asset)
		}
	}
	return result, nil
}

func (s ExecutionService) lookupPermission(name string) tooldef.Permission {
	registered, err := s.Registry.GetTool(name)
	if err != nil {
		return tooldef.PermissionExecute
	}
	return tooldef.NormalizePermission(registered.Permission())
}

func (s ExecutionService) executeOne(ctx context.Context, command toolcommand.Execution, call domainmessage.ToolCall, index int, createdAt time.Time) callExecution {
	callCreatedAt := createdAt.Add(time.Duration(index*2) * time.Nanosecond)
	resultCreatedAt := createdAt.Add(time.Duration(index*2+1) * time.Nanosecond)
	registeredTool, err := s.Registry.GetTool(call.Name)
	if err != nil {
		return s.failedCall(command, call, "tool_not_found", err, callCreatedAt, resultCreatedAt)
	}
	permission := tooldef.NormalizePermission(registeredTool.Permission())
	// PreToolUse 可拒绝调用或重写参数；Allow 只表示 Hook 放行，不能绕过权限检查。
	hookResult, err := s.beforeToolUse(ctx, toolcommand.HookEvent{SessionID: command.SessionID, Name: call.Name, Arguments: call.Arguments, Permission: permission})
	if err != nil {
		return s.failedCall(command, call, "hook_pre_failed", err, callCreatedAt, resultCreatedAt)
	}
	if strings.TrimSpace(hookResult.Arguments) != "" {
		call.Arguments = hookResult.Arguments
	}
	if command.Callbacks.OnToolCall != nil {
		command.Callbacks.OnToolCall(call.Name, call.Arguments)
	}
	entries := []domaintool.ExecutionEntry{ToolCallEntry(toolcommand.ToolCallEntry{SessionID: command.SessionID, Call: call, CreatedAt: callCreatedAt})}
	if !toolAllowed(command.AllowedTools, command.EnforceToolAllowlist, call.Name) {
		message := fmt.Sprintf("tool %s is not allowed for this subagent", call.Name)
		output := domaintool.FailedOutput(domaintool.ResultStatusDenied, "subagent_tool_denied", message)
		return s.finishedCall(ctx, command, call, permission, output, nil, entries, resultCreatedAt)
	}

	if hookResult.Decision == toolresult.HookDecisionDeny {
		err = fmt.Errorf("tool denied by hook: %s", hookResult.Message)
		output := domaintool.FailedOutput(domaintool.ResultStatusDenied, "hook_denied", "tool error: "+err.Error())
		return s.finishedCall(ctx, command, call, permission, output, err, entries, resultCreatedAt)
	}
	if !allowsExecutionInMode(permission, command.AgentMode, command.ForceChatMode) {
		message := fmt.Sprintf("permission denied: plan mode does not allow tool %s requiring %s", call.Name, permission)
		output := domaintool.FailedOutput(domaintool.ResultStatusDenied, "plan_mode_denied", message)
		return s.finishedCall(ctx, command, call, permission, output, nil, entries, resultCreatedAt)
	}

	permissionDecision := s.permissionService().Allow(toolcommand.Permission{
		Name: call.Name, Arguments: call.Arguments, Permission: permission, Mode: command.PermissionMode,
		WorkspaceRoot: command.WorkspaceRoot, Isolated: command.Isolated,
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
	return s.finishedCall(ctx, command, call, permission, output.Normalized(), toolErr, entries, resultCreatedAt)
}

func (s ExecutionService) failedCall(command toolcommand.Execution, call domainmessage.ToolCall, code string, err error, callCreatedAt, resultCreatedAt time.Time) callExecution {
	output := domaintool.FailedOutput(domaintool.ResultStatusFailed, code, "tool error: "+err.Error())
	if command.Callbacks.OnToolCall != nil {
		command.Callbacks.OnToolCall(call.Name, call.Arguments)
	}
	if command.Callbacks.OnToolResult != nil {
		command.Callbacks.OnToolResult(call.Name, call.Arguments, output)
	}
	return callExecution{
		call:    call,
		message: ToolResultMessage(call, output),
		entries: []domaintool.ExecutionEntry{
			ToolCallEntry(toolcommand.ToolCallEntry{SessionID: command.SessionID, Call: call, CreatedAt: callCreatedAt}),
			ToolResultEntry(toolcommand.ToolResultEntry{SessionID: command.SessionID, Call: call, Output: output, CreatedAt: resultCreatedAt}),
		},
	}
}

func (s ExecutionService) finishedCall(ctx context.Context, command toolcommand.Execution, call domainmessage.ToolCall, permission tooldef.Permission, output domaintool.ToolOutput, toolErr error, entries []domaintool.ExecutionEntry, resultCreatedAt time.Time) callExecution {
	output = output.Normalized()
	if command.Callbacks.OnToolResult != nil {
		command.Callbacks.OnToolResult(call.Name, call.Arguments, output)
	}
	s.afterToolUse(ctx, toolcommand.HookEvent{SessionID: command.SessionID, Name: call.Name, Arguments: call.Arguments, Permission: permission, Result: output.Content, Err: outputError(output, toolErr)})
	result := callExecution{
		call:    call,
		message: ToolResultMessage(call, output),
		entries: append(entries, ToolResultEntry(toolcommand.ToolResultEntry{SessionID: command.SessionID, Call: call, Output: output, CreatedAt: resultCreatedAt})),
	}
	if !output.Failed() && s.Assets != nil {
		if asset, ok := s.Assets.Extract(toolcommand.AssetExtraction{SessionID: command.SessionID, RequestID: command.RequestID, Call: call, Output: output, CreatedAt: resultCreatedAt}); ok {
			result.asset = asset
			result.hasAsset = true
		}
	}
	return result
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
