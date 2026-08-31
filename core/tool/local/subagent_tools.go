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
type SpawnAgentTool struct{ service subagentapi.Service }
type CheckAsyncTaskTool struct{ service subagentapi.Service }
type ListAsyncTasksTool struct{ service subagentapi.Service }
type ListAgentsTool struct{ service subagentapi.Service }
type WaitAgentTool struct{ service subagentapi.Service }
type CancelAsyncTaskTool struct{ service subagentapi.Service }
type InterruptAgentTool struct{ service subagentapi.Service }
type ApplyTaskChangesTool struct{ service subagentapi.Service }
type DiscardTaskChangesTool struct{ service subagentapi.Service }

func NewSubagentTools(service subagentapi.Service) []tooldef.Tool {
	return []tooldef.Tool{
		&ListSubagentDefinitionsTool{service: service},
		&StartAsyncTaskTool{service: service},
		&SpawnAgentTool{service: service},
		&CheckAsyncTaskTool{service: service},
		&ListAsyncTasksTool{service: service},
		&ListAgentsTool{service: service},
		&WaitAgentTool{service: service},
		&CancelAsyncTaskTool{service: service},
		&InterruptAgentTool{service: service},
		&ApplyTaskChangesTool{service: service},
		&DiscardTaskChangesTool{service: service},
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
		ParentTaskID: execution.TaskID,
		ParentRunID:  execution.RunID,
		PlanID:       execution.PlanID, StepID: execution.StepID,
		DefinitionID: input.DefinitionID, Instruction: input.Instruction, Title: input.Title,
		FallbackModelID: execution.ModelID, WorkspaceRoot: execution.WorkspaceRoot,
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return jsonToolOutput(taskView(result.Value, false))
}

// SpawnAgentTool is the Codex-compatible name for starting a child agent.
// It uses the existing definition registry while exposing thread-like
// metadata in the returned task summary.
func (tool *SpawnAgentTool) Name() string { return "spawn_agent" }
func (tool *SpawnAgentTool) Description() string {
	return "Create a child agent, start it asynchronously, and return its task identity immediately."
}
func (tool *SpawnAgentTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message":       map[string]any{"type": "string"},
			"task_name":     map[string]any{"type": "string"},
			"definition_id": map[string]any{"type": "string"},
			"agent_type":    map[string]any{"type": "string"},
			"model":         map[string]any{"type": "string"},
		},
		"required": []string{"message", "task_name"},
	}
}
func (tool *SpawnAgentTool) Permission() tooldef.Permission { return tooldef.PermissionExecute }
func (tool *SpawnAgentTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	var input struct {
		Message      string `json:"message"`
		TaskName     string `json:"task_name"`
		DefinitionID string `json:"definition_id"`
		AgentType    string `json:"agent_type"`
		Model        string `json:"model"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tooldef.ToolOutput{}, err
	}
	input.Message = strings.TrimSpace(input.Message)
	input.TaskName = strings.TrimSpace(input.TaskName)
	if input.Message == "" || input.TaskName == "" {
		return tooldef.ToolOutput{}, errors.New("spawn_agent requires message and task_name")
	}
	execution, ok := toolruntime.CurrentExecution(ctx)
	if !ok || strings.TrimSpace(execution.SessionID) == "" {
		return tooldef.ToolOutput{}, errors.New("subagent parent execution context is unavailable")
	}
	definitionID := strings.TrimSpace(input.DefinitionID)
	if definitionID == "" {
		definitionID = strings.TrimSpace(input.AgentType)
	}
	if definitionID == "" {
		// Keep the Codex shape where agent_type is optional. The safe built-in
		// researcher is the default; writable roles must be selected explicitly.
		definitionID = "researcher"
	}
	modelID := strings.TrimSpace(input.Model)
	if modelID == "" {
		modelID = execution.ModelID
	}
	result, err := tool.service.Start(ctx, subagentcommand.StartTask{
		ParentSessionID: execution.SessionID, CreatedRequestID: execution.RequestID,
		ParentTaskID: execution.TaskID,
		ParentRunID:  execution.RunID,
		PlanID:       execution.PlanID, StepID: execution.StepID,
		DefinitionID: definitionID, Instruction: input.Message, Title: input.TaskName,
		TaskName: input.TaskName, AgentNickname: strings.TrimSpace(input.AgentType),
		ModelID:         modelID,
		FallbackModelID: modelID, WorkspaceRoot: execution.WorkspaceRoot,
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

func (tool *ListAgentsTool) Name() string { return "list_agents" }
func (tool *ListAgentsTool) Description() string {
	return "List child agents belonging to the current parent session and their latest states."
}
func (tool *ListAgentsTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}}}
}
func (tool *ListAgentsTool) Permission() tooldef.Permission { return tooldef.PermissionRead }
func (tool *ListAgentsTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	var input struct {
		Limit int `json:"limit"`
	}
	if len(bytes.TrimSpace(args)) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return tooldef.ToolOutput{}, err
		}
	}
	execution, ok := toolruntime.CurrentExecution(ctx)
	if !ok || strings.TrimSpace(execution.SessionID) == "" {
		return tooldef.ToolOutput{}, errors.New("subagent parent execution context is unavailable")
	}
	result, err := tool.service.List(ctx, subagentcommand.ListTasks{ParentSessionID: execution.SessionID, Limit: input.Limit})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	agents := make([]taskToolView, 0, len(result.Items))
	for _, task := range result.Items {
		agents = append(agents, taskView(task, false))
	}
	return jsonToolOutput(map[string]any{"agents": agents})
}

func (tool *WaitAgentTool) Name() string { return "wait_agent" }
func (tool *WaitAgentTool) Description() string {
	return "Wait asynchronously for a child agent to reach a terminal state or until the timeout expires."
}
func (tool *WaitAgentTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"task_id":    map[string]any{"type": "string"},
		"timeout_ms": map[string]any{"type": "integer", "minimum": 100, "maximum": 600000},
	}, "required": []string{"task_id"}}
}
func (tool *WaitAgentTool) Permission() tooldef.Permission { return tooldef.PermissionRead }
func (tool *WaitAgentTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	var input struct {
		TaskID    string `json:"task_id"`
		TimeoutMS int64  `json:"timeout_ms"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tooldef.ToolOutput{}, err
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return tooldef.ToolOutput{}, errors.New("wait_agent requires task_id")
	}
	execution, ok := toolruntime.CurrentExecution(ctx)
	if !ok || strings.TrimSpace(execution.SessionID) == "" {
		return tooldef.ToolOutput{}, errors.New("subagent parent execution context is unavailable")
	}
	var timeout time.Duration
	if input.TimeoutMS > 0 {
		timeout = time.Duration(input.TimeoutMS) * time.Millisecond
	}
	result, err := tool.service.Wait(ctx, subagentcommand.WaitTask{
		TaskID: input.TaskID, ParentSessionID: execution.SessionID, ParentTaskID: execution.TaskID, Timeout: timeout,
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return jsonToolOutput(map[string]any{
		"task": taskView(result.Task, true), "timed_out": result.TimedOut, "sequence": result.Sequence,
	})
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
	execution, ok := toolruntime.CurrentExecution(ctx)
	if !ok || strings.TrimSpace(execution.SessionID) == "" {
		return tooldef.ToolOutput{}, errors.New("subagent parent execution context is unavailable")
	}
	result, err := tool.service.Cancel(ctx, subagentcommand.CancelTask{
		TaskID: taskID, ParentSessionID: execution.SessionID, Reason: "canceled by parent agent",
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return jsonToolOutput(taskView(result.Value, false))
}

func (tool *InterruptAgentTool) Name() string { return "interrupt_agent" }
func (tool *InterruptAgentTool) Description() string {
	return "Interrupt a running child agent and mark it canceled after the runtime receives the request."
}
func (tool *InterruptAgentTool) Schema() any                    { return taskIDSchema() }
func (tool *InterruptAgentTool) Permission() tooldef.Permission { return tooldef.PermissionExecute }
func (tool *InterruptAgentTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	taskID, err := decodeTaskID(args)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	execution, ok := toolruntime.CurrentExecution(ctx)
	if !ok || strings.TrimSpace(execution.SessionID) == "" {
		return tooldef.ToolOutput{}, errors.New("subagent parent execution context is unavailable")
	}
	result, err := tool.service.Cancel(ctx, subagentcommand.CancelTask{
		TaskID: taskID, ParentSessionID: execution.SessionID, Reason: "interrupted by parent agent",
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return jsonToolOutput(taskView(result.Value, true))
}

func (tool *ApplyTaskChangesTool) Name() string { return "apply_task_changes" }
func (tool *ApplyTaskChangesTool) Description() string {
	return "Apply the completed changes from an isolated child agent task to the parent workspace."
}
func (tool *ApplyTaskChangesTool) Schema() any                    { return taskIDSchema() }
func (tool *ApplyTaskChangesTool) Permission() tooldef.Permission { return tooldef.PermissionWrite }
func (tool *ApplyTaskChangesTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	taskID, err := decodeTaskID(args)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	execution, ok := toolruntime.CurrentExecution(ctx)
	if !ok || strings.TrimSpace(execution.SessionID) == "" {
		return tooldef.ToolOutput{}, errors.New("subagent parent execution context is unavailable")
	}
	result, err := tool.service.ApplyChanges(ctx, subagentcommand.ApplyTaskChanges{
		TaskID: taskID, ParentSessionID: execution.SessionID, RequestID: execution.RequestID,
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return jsonToolOutput(taskView(result.Value, true))
}

func (tool *DiscardTaskChangesTool) Name() string { return "discard_task_changes" }
func (tool *DiscardTaskChangesTool) Description() string {
	return "Discard the isolated workspace changes from a completed child agent task."
}
func (tool *DiscardTaskChangesTool) Schema() any                    { return taskIDSchema() }
func (tool *DiscardTaskChangesTool) Permission() tooldef.Permission { return tooldef.PermissionWrite }
func (tool *DiscardTaskChangesTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.service == nil {
		return tooldef.ToolOutput{}, errors.New("subagent service is not configured")
	}
	taskID, err := decodeTaskID(args)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	execution, ok := toolruntime.CurrentExecution(ctx)
	if !ok || strings.TrimSpace(execution.SessionID) == "" {
		return tooldef.ToolOutput{}, errors.New("subagent parent execution context is unavailable")
	}
	result, err := tool.service.DiscardChanges(ctx, subagentcommand.DiscardTaskChanges{
		TaskID: taskID, ParentSessionID: execution.SessionID,
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return jsonToolOutput(taskView(result.Value, true))
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
	ParentTaskID   string     `json:"parent_task_id,omitempty"`
	ParentRunID    string     `json:"parent_run_id,omitempty"`
	PlanID         string     `json:"plan_id,omitempty"`
	StepID         string     `json:"step_id,omitempty"`
	AgentPath      string     `json:"agent_path,omitempty"`
	AgentNickname  string     `json:"agent_nickname,omitempty"`
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
		TaskID: task.ID, ParentTaskID: task.ParentTaskID, ParentRunID: task.ParentRunID, PlanID: task.PlanID, StepID: task.StepID, AgentPath: task.AgentPath, AgentNickname: task.AgentNickname,
		Title: task.Title, DefinitionID: task.DefinitionID,
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
