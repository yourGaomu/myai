package service

import (
	"context"
	"strings"
	"sync"
	"unicode/utf8"

	agentrunapi "myai/core/application/agentrun/api"
	agentruncommand "myai/core/application/agentrun/command"
	domainagentrun "myai/core/domain/agentrun"
	modelport "myai/core/port/model"
)

const (
	maxReasoningEventBytes  = 1024 * 1024
	maxToolResultEventBytes = 256 * 1024
	maxToolArgumentBytes    = 64 * 1024
)

type runStreamRecorder struct {
	ctx       context.Context
	runID     string
	runs      agentrunapi.CommandService
	base      modelport.ChatStreamHandler
	onError   func(error)
	mu        sync.Mutex
	reasoning strings.Builder
	truncated bool
	event     domainagentrun.Event
}

func newRunStreamRecorder(ctx context.Context, runID string, runs agentrunapi.CommandService, base modelport.ChatStreamHandler, onError func(error)) *runStreamRecorder {
	return &runStreamRecorder{ctx: ctx, runID: runID, runs: runs, base: base, onError: onError}
}

func (r *runStreamRecorder) Handler() modelport.ChatStreamHandler {
	return modelport.ChatStreamHandler{
		CorrelationID:  r.base.CorrelationID,
		OnReasoning:    r.onReasoning,
		OnAnswer:       r.onAnswer,
		OnToolCall:     r.onToolCall,
		OnToolResult:   r.onToolResult,
		OnToolAsk:      r.onToolAsk,
		OnRunStarted:   r.base.OnRunStarted,
		OnRunEvent:     r.base.OnRunEvent,
		OnRunCompleted: r.base.OnRunCompleted,
	}
}

func (r *runStreamRecorder) Close() {
	r.flushReasoning()
}

func (r *runStreamRecorder) onReasoning(text string) {
	if text == "" {
		return
	}
	r.mu.Lock()
	delta, truncated := appendBounded(&r.reasoning, text, maxReasoningEventBytes)
	r.truncated = r.truncated || truncated
	if r.event.ID == "" && delta != "" {
		event, err := r.runs.Append(r.ctx, agentruncommand.Append{
			RunID: r.runID, Type: domainagentrun.EventTypeReasoning, Title: "Model reasoning",
			Content: delta, Truncated: r.truncated,
		})
		if err != nil {
			r.report(err)
		} else {
			r.event = event
			r.publish(event)
		}
	} else if r.event.ID != "" && delta != "" {
		eventDelta := r.event
		eventDelta.Content = delta
		eventDelta.Delta = true
		eventDelta.Truncated = r.truncated
		r.publish(eventDelta)
	}
	r.mu.Unlock()
	if r.base.OnReasoning != nil {
		r.base.OnReasoning(text)
	}
}

func (r *runStreamRecorder) onAnswer(text string) {
	r.flushReasoning()
	if r.base.OnAnswer != nil {
		r.base.OnAnswer(text)
	}
}

func (r *runStreamRecorder) onToolCall(name string, arguments string) {
	r.flushReasoning()
	boundedArguments, truncated := boundedText(arguments, maxToolArgumentBytes)
	r.record(agentruncommand.Append{
		RunID: r.runID, Type: domainagentrun.EventTypeToolCall, Title: "Tool started",
		ToolName: name, Arguments: boundedArguments, Status: "running", Truncated: truncated,
	})
	if r.base.OnToolCall != nil {
		r.base.OnToolCall(name, arguments)
	}
}

func (r *runStreamRecorder) onToolResult(result modelport.ToolResultEvent) {
	r.flushReasoning()
	output := result.Output.Normalized()
	content, contentTruncated := boundedText(output.Content, maxToolResultEventBytes)
	arguments, argumentsTruncated := boundedText(result.Arguments, maxToolArgumentBytes)
	r.record(agentruncommand.Append{
		RunID: r.runID, Type: domainagentrun.EventTypeToolResult, Title: "Tool finished",
		ToolName: result.Name, Arguments: arguments, Content: content, Status: string(output.Status),
		ErrorCode: output.ErrorCode, ErrorMessage: output.ErrorMessage,
		Truncated: output.Truncated || contentTruncated || argumentsTruncated,
	})
	if r.base.OnToolResult != nil {
		r.base.OnToolResult(result)
	}
}

func (r *runStreamRecorder) onToolAsk(request modelport.ToolPermissionRequest) bool {
	r.flushReasoning()
	arguments, truncated := boundedText(request.Arguments, maxToolArgumentBytes)
	r.record(agentruncommand.Append{
		RunID: r.runID, Type: domainagentrun.EventTypePermission, Title: "Permission required",
		ToolName: request.Name, Arguments: arguments, Status: "waiting", Truncated: truncated,
	})
	allowed := false
	if r.base.OnToolAsk != nil {
		allowed = r.base.OnToolAsk(request)
	}
	status := "denied"
	if allowed {
		status = "allowed"
	}
	r.record(agentruncommand.Append{
		RunID: r.runID, Type: domainagentrun.EventTypePermission, Title: "Permission resolved",
		ToolName: request.Name, Arguments: arguments, Status: status, Truncated: truncated,
	})
	return allowed
}

func (r *runStreamRecorder) flushReasoning() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.event.ID == "" {
		r.reasoning.Reset()
		r.truncated = false
		return
	}
	content := r.reasoning.String()
	ctx := context.WithoutCancel(r.ctx)
	if err := r.runs.ReplaceEventContent(ctx, agentruncommand.ReplaceEventContent{
		RunID: r.runID, EventID: r.event.ID, Content: content, Truncated: r.truncated,
	}); err != nil {
		r.report(err)
	}
	r.reasoning.Reset()
	r.truncated = false
	r.event = domainagentrun.Event{}
}

func (r *runStreamRecorder) record(command agentruncommand.Append) {
	event, err := r.runs.Append(r.ctx, command)
	if err != nil {
		r.report(err)
		return
	}
	r.publish(event)
}

func (r *runStreamRecorder) publish(event domainagentrun.Event) {
	if r.base.OnRunEvent != nil {
		r.base.OnRunEvent(event)
	}
}

func (r *runStreamRecorder) report(err error) {
	if err != nil && r.onError != nil {
		r.onError(err)
	}
}

func boundedText(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end], true
}

func appendBounded(builder *strings.Builder, value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		builder.WriteString(value)
		return value, false
	}
	remaining := maxBytes - builder.Len()
	if remaining <= 0 {
		return "", value != ""
	}
	part, truncated := boundedText(value, remaining)
	builder.WriteString(part)
	return part, truncated
}
