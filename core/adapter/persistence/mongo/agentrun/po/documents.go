package po

import "time"

type RunDocument struct {
	ID           string     `bson:"_id"`
	RequestID    string     `bson:"request_id,omitempty"`
	SessionID    string     `bson:"session_id"`
	Kind         string     `bson:"kind"`
	Title        string     `bson:"title,omitempty"`
	Reason       string     `bson:"reason,omitempty"`
	Status       string     `bson:"status"`
	CurrentStep  int        `bson:"current_step,omitempty"`
	TotalSteps   int        `bson:"total_steps,omitempty"`
	LastSequence int64      `bson:"last_sequence,omitempty"`
	ErrorMessage string     `bson:"error_message,omitempty"`
	StartedAt    time.Time  `bson:"started_at"`
	FinishedAt   *time.Time `bson:"finished_at,omitempty"`
}

type EventDocument struct {
	ID           string    `bson:"_id"`
	RunID        string    `bson:"run_id"`
	SessionID    string    `bson:"session_id"`
	Sequence     int64     `bson:"sequence"`
	Type         string    `bson:"type"`
	Title        string    `bson:"title,omitempty"`
	Content      string    `bson:"content,omitempty"`
	ToolName     string    `bson:"tool_name,omitempty"`
	Arguments    string    `bson:"arguments,omitempty"`
	Status       string    `bson:"status,omitempty"`
	ErrorCode    string    `bson:"error_code,omitempty"`
	ErrorMessage string    `bson:"error_message,omitempty"`
	Truncated    bool      `bson:"truncated,omitempty"`
	CurrentStep  int       `bson:"current_step,omitempty"`
	TotalSteps   int       `bson:"total_steps,omitempty"`
	CreatedAt    time.Time `bson:"created_at"`
}
