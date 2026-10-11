package service

import (
	"context"

	domaintool "myai/core/domain/tool"
	modelport "myai/core/port/model"
	subagentport "myai/core/port/subagent"
)

// taskStream translates the existing chat stream callbacks into the ordered
// task event log. The runner remains independent of the application service;
// all identity and persistence concerns stay at this boundary.
func (service *Service) taskStream(ctx context.Context, taskID string, runID string) modelport.ChatStreamHandler {
	return modelport.ChatStreamHandler{
		CorrelationID: runID,
		OnReasoning: func(text string) {
			service.publishRuntimeEvent(ctx, taskID, runID, subagentport.TaskEventKindReasoning, text, "", "", "", "", "", true)
		},
		OnAnswer: func(text string) {
			service.publishRuntimeEvent(ctx, taskID, runID, subagentport.TaskEventKindAnswer, text, "", "", "", "", "", true)
		},
		OnToolCallEvent: func(event modelport.ToolCallEvent) {
			// 1. 用调用 ID 关联开始与结束事件，同名工具并发时也不会错配。
			service.publishTaskStreamEvent(ctx, taskID, runID, subagentport.TaskEvent{
				Kind: subagentport.TaskEventKindToolCall, ToolCallID: event.ToolCallID,
				ToolName: event.Name, Arguments: event.Arguments, Status: "running",
			})
		},
		OnToolResult: func(event modelport.ToolResultEvent) {
			output := event.Output.Normalized()
			service.publishTaskStreamEvent(ctx, taskID, runID, subagentport.TaskEvent{
				Kind: subagentport.TaskEventKindToolResult, ToolCallID: event.ToolCallID,
				Content: output.Content, ToolName: event.Name, Arguments: event.Arguments,
				Status: string(output.Status), ErrorCode: output.ErrorCode, ErrorMessage: output.ErrorMessage, Truncated: output.Truncated,
			})
		},
		OnAsset: func(asset domaintool.SharedAsset) {
			// 2. 图片和附件只推送资源引用，二进制内容仍由文件服务提供。
			service.publishTaskStreamEvent(ctx, taskID, runID, subagentport.TaskEvent{
				Kind: subagentport.TaskEventKindAsset, ToolCallID: asset.ToolCallID,
				ToolName: asset.ToolName, Assets: []domaintool.SharedAsset{asset},
			})
		},
		OnToolAsk: func(request modelport.ToolPermissionRequest) bool {
			// 2.1 后台子任务没有交互审批通道时明确拒绝，并让详情页显示原因。
			// 不能仅发一个 waiting 事件，让前端误以为已经存在可操作的审批。
			service.publishTaskStreamEvent(ctx, taskID, runID, subagentport.TaskEvent{
				Kind: subagentport.TaskEventKindPermission, ToolCallID: request.ToolCallID,
				ToolName: request.Name, Arguments: request.Arguments, Status: "denied",
				ErrorCode: "approval_unavailable", ErrorMessage: "后台子代理暂不支持交互审批，本次调用已拒绝。",
			})
			return false
		},
	}
}

func (service *Service) publishRuntimeEvent(ctx context.Context, taskID string, runID string, kind string, content string, toolName string, arguments string, status string, errorCode string, errorMessage string, delta bool) {
	service.publishTaskStreamEvent(ctx, taskID, runID, subagentport.TaskEvent{
		Kind: kind, Content: content, ToolName: toolName, Arguments: arguments,
		Status: status, ErrorCode: errorCode, ErrorMessage: errorMessage, Delta: delta,
	})
}

func (service *Service) publishTaskStreamEvent(ctx context.Context, taskID, runID string, event subagentport.TaskEvent) {
	if service == nil || service.Events == nil || service.Tasks == nil {
		return
	}
	publisher, ok := service.Events.(subagentport.TaskEventPublisher)
	if !ok {
		return
	}
	task, err := service.Tasks.GetTask(context.Background(), taskID)
	if err != nil {
		service.reportError(err)
		return
	}
	// 3. 每条事件绑定产生它的 Run，旧 Run 的迟到通知不覆盖当前任务快照。
	if task.CurrentRunID != runID {
		return
	}
	event.Task, event.RunID = task, runID
	publisher.PublishTaskEvent(ctx, event)
}
