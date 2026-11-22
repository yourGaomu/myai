package executor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	agentrunruntime "myai/core/application/agentrun/runtime"
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	toolcommand "myai/core/application/tool/command"
	toolport "myai/core/application/tool/port"
	toolresult "myai/core/application/tool/result"
	toolruntimeRegistry "myai/core/application/tool/runtime"
	toolservice "myai/core/application/tool/service"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/hook"
	modelport "myai/core/port/model"
	"myai/core/session"
	"myai/core/tool/local"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type Executor struct {
	// Executor 把 generation 层命令转换为 tool 应用命令，并把流式回调桥接到手机协议。
	Registry toolport.Registry
	Hooks    toolport.HookBridge
}

func (e Executor) Execute(ctx context.Context, command generationcommand.ToolExecution) (generationresult.ToolExecution, error) {
	if command.Session == nil {
		return generationresult.ToolExecution{}, errors.New("session is nil")
	}
	ctx = toolruntime.WithRAGSettings(ctx, command.Session.RAGSettings)
	if command.Stream.OnUserQuestion != nil {
		ctx = toolruntime.WithUserQuestionHandler(ctx, command.Stream.OnUserQuestion)
	}
	metadata := agentrunruntime.MetadataFrom(ctx)
	ctx = toolruntime.WithExecution(ctx, toolruntime.Execution{
		SessionID: command.Session.ID, RequestID: command.RequestID,
		ParentSessionID: command.Session.ParentSessionID,
		RunID:           metadata.RunID, ParentRunID: metadata.ParentRunID, PlanID: metadata.PlanID, StepID: metadata.StepID,
		// Metadata.TaskID identifies the agent currently executing this tool
		// call. Session.ParentTaskID is the task that created the session and
		// must only be used for the child task's parent relationship. Using it
		// here made nested spawn_agent calls attach to the grandparent.
		TaskID:        metadata.TaskID,
		WorkspaceRoot: command.Session.WorkspaceRoot, SandboxID: command.Session.WorkspaceSandboxID,
		ModelID: command.Session.Model,
	})

	result, err := toolservice.ExecutionService{
		Registry: e.Registry,
		Invoker:  builtinInvoker{},
		Hooks:    e.Hooks,
		Assets:   SharedAssetExtractor{},
	}.Execute(ctx, toolcommand.Execution{
		SessionID:            command.Session.ID,
		AgentMode:            session.NormalizeAgentMode(command.Session.AgentMode),
		PermissionMode:       session.NormalizePermissionMode(command.Session.PermissionMode),
		ForceChatMode:        command.ForceChatMode,
		RequestID:            command.RequestID,
		WorkspaceRoot:        command.Session.WorkspaceRoot,
		Isolated:             e.sandboxedShell(ctx, command.Session.WorkspaceSandboxID),
		Calls:                command.Calls,
		PermissionCeiling:    tooldef.ClonePermissionCeiling(command.Session.PermissionCeiling),
		AllowedTools:         append([]string(nil), command.Session.AllowedTools...),
		EnforceToolAllowlist: command.Session.EnforceToolAllowlist,
		Callbacks:            callbacksFromStream(command.Stream),
	})
	if err != nil {
		return generationresult.ToolExecution{}, err
	}
	var budgetErr error
	result.Messages, result.Entries, budgetErr = limitToolResultsForRequest(
		result.Messages,
		result.Entries,
		promptToolResultBudgetForRequest(command, result.Calls),
	)

	var imageReads map[string]string
	if budgetErr == nil && len(result.ImageReads()) > 0 {
		for _, message := range result.Messages {
			read, ok := message.FirstToolResult()
			if !ok || read.Status != domaintool.ResultStatusSuccess || read.Truncated || read.PromptTruncated {
				continue
			}
			if assetID := result.ImageReads()[read.ToolCallID]; assetID != "" {
				if imageReads == nil {
					imageReads = make(map[string]string)
				}
				imageReads[read.ToolCallID] = string(assetID)
			}
		}
	}
	return (generationresult.ToolExecution{
		Calls:    result.Calls,
		Messages: result.Messages,
		Entries:  result.Entries,
		Assets:   result.Assets,
	}).WithImageReads(imageReads), budgetErr
}

type builtinInvoker struct{}

func (builtinInvoker) Invoke(ctx context.Context, registered tooldef.Tool, args json.RawMessage) (domaintool.ToolOutput, tooldef.ImageAssetID, error) {
	// A same-named tool, even one with a matching method, cannot attest a read.
	if readAsset, ok := registered.(*local.ReadAssetTool); ok {
		return readAsset.CallWithImageAssetID(ctx, args)
	}
	output, err := registered.Call(ctx, args)
	return output, "", err
}

func (e Executor) sandboxedShell(ctx context.Context, sandboxID string) bool {
	id := strings.TrimSpace(sandboxID)
	if id == "" || strings.HasPrefix(strings.ToLower(id), "snapshot:") {
		return false
	}
	registry := e.Registry
	if snapshot := toolruntimeRegistry.RegistryFrom(ctx); snapshot != nil {
		registry = snapshot
	}
	if registry == nil {
		return false
	}
	registered, err := registry.GetTool("shell")
	if err != nil {
		return false
	}
	shell, ok := registered.(tooldef.SandboxedShell)
	return ok && shell.IsSandboxedShell(ctx)
}

func callbacksFromStream(stream modelport.ChatStreamHandler) toolcommand.ExecutionCallbacks {
	// 1. 优先使用带 ID 的回调；一次调用只发布一次开始通知。
	legacyCall := stream.OnToolCall
	if stream.OnToolCallEvent != nil {
		legacyCall = nil
	}
	return toolcommand.ExecutionCallbacks{
		OnToolCall: legacyCall,
		OnToolCallIdentified: func(call domainmessage.ToolCall) {
			if stream.OnToolCallEvent != nil {
				stream.OnToolCallEvent(modelport.ToolCallEvent{ToolCallID: call.ID, Name: call.Name, Arguments: call.Arguments})
			}
		},
		OnToolResultIdentified: func(call domainmessage.ToolCall, output domaintool.ToolOutput) {
			if stream.OnToolResult != nil {
				stream.OnToolResult(modelport.ToolResultEvent{ToolCallID: call.ID, Name: call.Name, Arguments: call.Arguments, Output: output.Normalized()})
			}
		},
		OnAsset:   stream.OnAsset,
		OnToolAsk: permissionAskFromStream(stream),
	}
}

func permissionAskFromStream(stream modelport.ChatStreamHandler) toolcommand.PermissionAskFunc {
	if stream.OnToolAsk == nil {
		return nil
	}
	return func(request toolcommand.PermissionRequest) bool {
		return stream.OnToolAsk(modelport.ToolPermissionRequest{
			ToolCallID:          request.ToolCallID,
			Name:                request.Name,
			Arguments:           request.Arguments,
			Permission:          request.Permission,
			Mode:                string(request.Mode),
			PermissionIdentity:  request.PermissionIdentity,
			RequireFreshConsent: request.RequireFreshConsent,
		})
	}
}

type HookBridge struct {
	// HookBridge 隔离 core/hook 的事件对象，应用层只依赖自己的 HookBridge port。
	Hooks       *hook.Manager
	OnPostError func(error)
}

func (b HookBridge) BeforeToolUse(ctx context.Context, event toolcommand.HookEvent) (toolresult.Hook, error) {
	if b.Hooks == nil {
		return toolresult.Hook{Decision: toolresult.HookDecisionContinue}, nil
	}
	result, err := b.Hooks.PreToolUse(ctx, hook.Event{
		SessionID:     event.SessionID,
		ToolName:      event.Name,
		ToolArguments: event.Arguments,
		Permission:    string(event.Permission),
		Reason:        "tool execution",
	})
	if err != nil {
		return toolresult.Hook{}, err
	}
	return toolresult.Hook{
		Decision:  hookDecisionToToolDecision(result.Decision),
		Arguments: result.Arguments,
		Message:   result.Message,
	}, nil
}

func (b HookBridge) AfterToolUse(ctx context.Context, event toolcommand.HookEvent) {
	if b.Hooks == nil {
		return
	}
	errText := ""
	if event.Err != nil {
		errText = event.Err.Error()
	}
	if err := b.Hooks.Emit(ctx, hook.Event{
		Type:          hook.EventPostToolUse,
		SessionID:     event.SessionID,
		ToolName:      event.Name,
		ToolArguments: event.Arguments,
		Permission:    string(event.Permission),
		Result:        event.Result,
		Error:         errText,
		Reason:        "tool execution completed",
	}); err != nil {
		b.reportPostError(err)
	}
}

func (b HookBridge) reportPostError(err error) {
	if err == nil || b.OnPostError == nil {
		return
	}
	b.OnPostError(err)
}

func hookDecisionToToolDecision(decision hook.Decision) toolresult.HookDecision {
	switch decision {
	case hook.DecisionAllow:
		return toolresult.HookDecisionAllow
	case hook.DecisionAsk:
		return toolresult.HookDecisionAsk
	case hook.DecisionDeny:
		return toolresult.HookDecisionDeny
	default:
		return toolresult.HookDecisionContinue
	}
}
