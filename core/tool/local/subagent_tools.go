package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	subagentapi "myai/core/application/subagent/api"
	subagentcommand "myai/core/application/subagent/command"
	domainsubagent "myai/core/domain/subagent"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type ListSubagentDefinitionsTool struct{ service subagentapi.Service }
type StartAsyncTaskTool struct{ service subagentapi.Service }
type CheckAsyncTaskTool struct{ service subagentapi.Service }
type ListAsyncTasksTool struct{ service subagentapi.Service }
type CancelAsyncTaskTool struct{ service subagentapi.Service }

func NewSubagentTools(service subagentapi.Service) []tooldef.Tool {
	return []tooldef.Tool{
		&ListSubagentDefinitionsTool{service: service},
		&StartAsyncTaskTool{service: service},
		&CheckAsyncTaskTool{service: service},
		&ListAsyncTasksTool{service: service},
		&CancelAsyncTaskTool{service: service},
	}
}

func (tool *ListSubagentDefinitionsTool) Name() string { return "list_subagent_definitions" }
func (tool *ListSubagentDefinitionsTool) Description() string {
	return "List available subagent definitions and their capabilities before starting a background task."
}
func (tool *ListSubagentDefinitionsTool) Schema() any { return emptyObjectSchema() }
func (tool *ListSubagentDefinitionsTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}
func (tool *ListSubagentDefinitionsTool) Call(ctx context.Context, _ json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	result, err := tool.service.ListDefinitions(ctx)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	items := make([]definitionToolView, 0, len(result.Items))
	for _, definition := range result.Items {
		if definition.Deleted || !definition.Enabled {
			continue
		}
		items = append(items, definitionView(definition))
	}
	return jsonToolOutput(map[string]any{"definitions": items})
}

func (tool *StartAsyncTaskTool) Name() string { return "start_async_task" }
func (tool *StartAsyncTaskTool) Description() string {
	return "Start an independent background subagent task and return immediately with a task_id. Do not poll it in the same model request."
}
func (tool *StartAsyncTaskTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"definition_id": map[string]any{"type": "string", "description": "ID returned by list_subagent_definitions."},
			"instruction":   map[string]any{"type": "string", "description": "A self-contained instruction for the subagent."},
			"title":         map[string]any{"type": "string", "description": "Short user-facing task title."},
		},
		"required": []string{"definition_id", "instruction"},
	}
}
func (tool *StartAsyncTaskTool) Permission() tooldef.Permission { return tooldef.PermissionExecute }
func (tool *StartAsyncTaskTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	var input struct {
		DefinitionID string `json:"definition_id"`
		Instruction  string `json:"instruction"`
		Title        string `json:"title"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tooldef.ToolOutput{}, err
	}
	execution, ok := toolruntime.CurrentExecution(ctx)
	if !ok || strings.TrimSpace(execution.SessionID) == "" {
		return tooldef.ToolOutput{}, errors.New("subagent parent execution context is unavailable")
	}
	result, err := tool.service.Start(ctx, subagentcommand.StartTask{
		ParentSessionID: execution.SessionID, CreatedRequestID: execution.RequestID,
		DefinitionID: input.DefinitionID, Instruction: input.Instruction, Title: input.Title,
		FallbackModelID: execution.ModelID, WorkspaceRoot: execution.WorkspaceRoot,
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return jsonToolOutput(taskView(result.Value, false))
}

func (tool *CheckAsyncTaskTool) Name() string { return "check_async_task" }
func (tool *CheckAsyncTaskTool) Description() string {
	return "Get the state and final result of a background subagent task. Never call this in the request that started the task."
}
func (tool *CheckAsyncTaskTool) Schema() any                    { return taskIDSchema() }
func (tool *CheckAsyncTaskTool) Permission() tooldef.Permission { return tooldef.PermissionRead }
func (tool *CheckAsyncTaskTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	taskID, err := decodeTaskID(args)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	execution, _ := toolruntime.CurrentExecution(ctx)
	result, err := tool.service.Check(ctx, subagentcommand.CheckTask{
		TaskID: taskID, ParentSessionID: execution.SessionID, RequestID: execution.RequestID,
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return jsonToolOutput(taskView(result.Value, true))
}

func (tool *ListAsyncTasksTool) Name() string { return "list_async_tasks" }
func (tool *ListAsyncTasksTool) Description() string {
	return "List background subagent tasks belonging to the current parent session."
}
func (tool *ListAsyncTasksTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
	}}
}
func (tool *ListAsyncTasksTool) Permission() tooldef.Permission { return tooldef.PermissionRead }
func (tool *ListAsyncTasksTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	var input struct {
		Limit int `json:"limit"`
	}
	if len(bytes.TrimSpace(args)) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return tooldef.ToolOutput{}, fmt.Errorf("decode list_async_tasks arguments: %w", err)
		}
	}
	execution, ok := toolruntime.CurrentExecution(ctx)
	if !ok || execution.SessionID == "" {
		return tooldef.ToolOutput{}, errors.New("subagent parent execution context is unavailable")
	}
	result, err := tool.service.List(ctx, subagentcommand.ListTasks{ParentSessionID: execution.SessionID, Limit: input.Limit})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	items := make([]taskToolView, 0, len(result.Items))
	for _, task := range result.Items {
		items = append(items, taskView(task, false))
	}
	return jsonToolOutput(map[string]any{"tasks": items})
}

func (tool *CancelAsyncTaskTool) Name() string { return "cancel_async_task" }
func (tool *CancelAsyncTaskTool) Description() string {
	return "Cancel a queued or running background subagent task."
}
func (tool *CancelAsyncTaskTool) Schema() any                    { return taskIDSchema() }
func (tool *CancelAsyncTaskTool) Permission() tooldef.Permission { return tooldef.PermissionExecute }
func (tool *CancelAsyncTaskTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	taskID, err := decodeTaskID(args)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	execution, _ := toolruntime.CurrentExecution(ctx)
	result, err := tool.service.Cancel(ctx, subagentcommand.CancelTask{
		TaskID: taskID, ParentSessionID: execution.SessionID, Reason: "canceled by parent agent",
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return jsonToolOutput(taskView(result.Value, false))
}

type definitionToolView struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	CapabilityMode string   `json:"capability_mode"`
	IsolationMode  string   `json:"isolation_mode"`
	AllowedTools   []string `json:"allowed_tools,omitempty"`
}

type taskToolView struct {
	TaskID         string     `json:"task_id"`
	Title          string     `json:"title"`
	DefinitionID   string     `json:"definition_id"`
	ChildSessionID string     `json:"child_session_id,omitempty"`
	Status         string     `json:"status"`
	Result         string     `json:"result,omitempty"`
	Error          string     `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

func definitionView(definition domainsubagent.Definition) definitionToolView {
	return definitionToolView{
		ID: definition.ID, Name: definition.Name, Description: definition.Description,
		CapabilityMode: string(definition.CapabilityMode), IsolationMode: string(definition.IsolationMode),
		AllowedTools: append([]string(nil), definition.AllowedTools...),
	}
}

func taskView(task domainsubagent.Task, includeResult bool) taskToolView {
	view := taskToolView{
		TaskID: task.ID, Title: task.Title, DefinitionID: task.DefinitionID,
		ChildSessionID: task.ChildSessionID, Status: string(task.Status), Error: task.ErrorMessage,
		CreatedAt: task.CreatedAt, StartedAt: task.StartedAt, CompletedAt: task.CompletedAt,
	}
	if includeResult || task.Terminal() {
		view.Result = task.Result
	}
	return view
}

func emptyObjectSchema() any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func taskIDSchema() any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"task_id": map[string]any{"type": "string"}},
		"required":   []string{"task_id"},
	}
}

func decodeTaskID(args json.RawMessage) (string, error) {
	var input struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	input.TaskID = strings.TrimSpace(input.TaskID)
	if input.TaskID == "" {
		return "", errors.New("subagent task id is required")
	}
	return input.TaskID, nil
}

func jsonToolOutput(value any) (tooldef.ToolOutput, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return tooldef.SuccessOutput(string(content)), nil
}
