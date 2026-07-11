package executor

import (
	"context"
	"errors"

	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	toolcommand "myai/core/application/tool/command"
	toolport "myai/core/application/tool/port"
	toolresult "myai/core/application/tool/result"
	toolservice "myai/core/application/tool/service"
	"myai/core/hook"
	modelport "myai/core/port/model"
	"myai/core/session"
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

	result, err := toolservice.ExecutionService{
		Registry: e.Registry,
		Hooks:    e.Hooks,
		Assets:   SharedAssetExtractor{},
	}.Execute(ctx, toolcommand.Execution{
		SessionID:      command.Session.ID,
		PermissionMode: session.NormalizePermissionMode(command.Session.PermissionMode),
		RequestID:      command.RequestID,
		Calls:          command.Calls,
		Callbacks:      callbacksFromStream(command.Stream),
	})
	if err != nil {
		return generationresult.ToolExecution{}, err
	}

	return generationresult.ToolExecution{
		Messages: result.Messages,
		Entries:  result.Entries,
		Assets:   result.Assets,
	}, nil
}

func callbacksFromStream(stream modelport.ChatStreamHandler) toolcommand.ExecutionCallbacks {
	return toolcommand.ExecutionCallbacks{
		OnToolCall:   stream.OnToolCall,
		OnToolResult: stream.OnToolResult,
		OnToolAsk:    permissionAskFromStream(stream),
	}
}

func permissionAskFromStream(stream modelport.ChatStreamHandler) toolcommand.PermissionAskFunc {
	if stream.OnToolAsk == nil {
		return nil
	}
	return func(request toolcommand.PermissionRequest) bool {
		return stream.OnToolAsk(modelport.ToolPermissionRequest{
			Name:       request.Name,
			Arguments:  request.Arguments,
			Permission: request.Permission,
			Mode:       string(request.Mode),
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
	case hook.DecisionDeny:
		return toolresult.HookDecisionDeny
	default:
		return toolresult.HookDecisionContinue
	}
}
