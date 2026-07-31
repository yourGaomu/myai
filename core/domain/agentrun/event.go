package agentrun

import (
	"errors"
	"strings"
	"time"
)

type EventType string

const (
	EventTypeProgress   EventType = "progress"
	EventTypeReasoning  EventType = "reasoning"
	EventTypeToolCall   EventType = "tool_call"
	EventTypeToolResult EventType = "tool_result"
	EventTypePermission EventType = "permission"
	EventTypePlanUpdate EventType = "plan_update"
	EventTypeCompleted  EventType = "completed"
	EventTypePaused     EventType = "paused"
	EventTypeFailed     EventType = "failed"
	EventTypeCanceled   EventType = "canceled"
)

type Event struct {
	ID           string
	RunID        string
	SessionID    string
	Sequence     int64
	Type         EventType
	Title        string
	Content      string
	ToolName     string
	Arguments    string
	Status       string
	ErrorCode    string
	ErrorMessage string
	Truncated    bool
	Delta        bool
	CurrentStep  int
	TotalSteps   int
	CreatedAt    time.Time
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.New("agent run event id is empty")
	}
	if strings.TrimSpace(e.RunID) == "" {
		return errors.New("agent run event run id is empty")
	}
	if strings.TrimSpace(e.SessionID) == "" {
		return errors.New("agent run event session id is empty")
	}
	if e.Sequence <= 0 {
		return errors.New("agent run event sequence must be positive")
	}
	if e.Type == "" {
		return errors.New("agent run event type is empty")
	}
	if e.CreatedAt.IsZero() {
		return errors.New("agent run event created at is empty")
	}
	return nil
}
