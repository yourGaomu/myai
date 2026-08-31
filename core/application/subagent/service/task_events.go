package service

import (
	"context"

	domainsubagent "myai/core/domain/subagent"
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
		OnToolCall: func(name string, arguments string) {
			service.publishRuntimeEvent(ctx, taskID, runID, subagentport.TaskEventKindToolCall, "", name, arguments, "running", "", "", false)
		},
		OnToolResult: func(event modelport.ToolResultEvent) {
			output := event.Output.Normalized()
			service.publishRuntimeEvent(ctx, taskID, runID, subagentport.TaskEventKindToolResult, output.Content, event.Name, event.Arguments, string(output.Status), output.ErrorCode, output.ErrorMessage, false)
		},
	}
}

func (service *Service) publishRuntimeEvent(ctx context.Context, taskID string, runID string, kind string, content string, toolName string, arguments string, status string, errorCode string, errorMessage string, delta bool) {
	if service == nil || service.Events == nil || service.Tasks == nil {
		return
	}
	publisher, ok := service.Events.(subagentport.TaskEventPublisher)
	if !ok {
		return
	}
	task, err := service.Tasks.GetTask(context.Background(), taskID)
	if err != nil {
		return
	}
	publisher.PublishTaskEvent(ctx, subagentport.TaskEvent{
		Kind: kind, Task: domainsubagent.CloneTask(task), RunID: runID, Content: content,
		ToolName: toolName, Arguments: arguments, Status: status, ErrorCode: errorCode,
		ErrorMessage: errorMessage, Delta: delta,
	})
}
