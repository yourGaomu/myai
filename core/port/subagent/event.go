package subagent

import (
	"time"

	domainsubagent "myai/core/domain/subagent"
	domaintool "myai/core/domain/tool"
)

const (
	TaskEventKindUpdated    = "task.updated"
	TaskEventKindStarted    = "task.started"
	TaskEventKindReasoning  = "task.reasoning"
	TaskEventKindAnswer     = "task.answer"
	TaskEventKindToolCall   = "task.tool_call"
	TaskEventKindToolResult = "task.tool_result"
	TaskEventKindPermission = "task.permission"
	TaskEventKindWaiting    = "task.waiting"
	TaskEventKindAsset      = "task.asset_created"
	TaskEventKindCompleted  = "task.completed"
	TaskEventKindCanceled   = "task.canceled"
	TaskEventKindFailed     = "task.failed"
)

// TaskEvent is the ordered, parent-scoped notification emitted by the
// subagent runtime. Sequence makes reconnects and replay deterministic.
type TaskEvent struct {
	Sequence     uint64
	Kind         string
	Task         domainsubagent.Task
	RunID        string
	ToolCallID   string
	Assets       []domaintool.SharedAsset
	Content      string
	ToolName     string
	Arguments    string
	Status       string
	ErrorCode    string
	ErrorMessage string
	Truncated    bool
	Delta        bool
	EmittedAt    time.Time
}

func CloneTaskEvent(event TaskEvent) TaskEvent {
	event.Task = domainsubagent.CloneTask(event.Task)
	event.Assets = domaintool.CloneSharedAssets(event.Assets)
	return event
}
