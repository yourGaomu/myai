package protocol

import "time"

type SubagentTaskDetailPayload struct {
	TaskID         string `json:"task_id"`
	AfterMessageID string `json:"after_message_id,omitempty"`
	MessageLimit   int    `json:"message_limit,omitempty"`
}

type SubagentRunSummary struct {
	ID           string     `json:"id"`
	Sequence     int        `json:"sequence"`
	Instruction  string     `json:"instruction"`
	Status       string     `json:"status"`
	Result       string     `json:"result,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type SubagentTaskDetailResultPayload struct {
	Task             SubagentTaskSummary     `json:"task"`
	ReadOnly         bool                    `json:"read_only"`
	SessionReady     bool                    `json:"session_ready"`
	EventSequence    uint64                  `json:"event_sequence"`
	Messages         []SessionHistoryMessage `json:"messages"`
	NextMessageID    string                  `json:"next_message_id,omitempty"`
	HasMoreMessages  bool                    `json:"has_more_messages"`
	FullSyncRequired bool                    `json:"full_sync_required"`
	Runs             []SubagentRunSummary    `json:"runs"`
	AgentRuns        []AgentRunSnapshot      `json:"agent_runs"`
	Assets           []AssetSummary          `json:"assets"`
}

type SubagentTaskEventsPayload struct {
	TaskID          string `json:"task_id,omitempty"`
	RunID           string `json:"run_id,omitempty"`
	AfterSequence   uint64 `json:"after_sequence,omitempty"`
	ThroughSequence uint64 `json:"through_sequence,omitempty"`
	Limit           int    `json:"limit,omitempty"`
}

type SubagentTaskEventsResultPayload struct {
	SessionID       string                      `json:"session_id"`
	TaskID          string                      `json:"task_id,omitempty"`
	RunID           string                      `json:"run_id,omitempty"`
	Events          []SubagentTaskResultPayload `json:"events"`
	NextSequence    uint64                      `json:"next_sequence"`
	ThroughSequence uint64                      `json:"through_sequence"`
	HasMore         bool                        `json:"has_more"`
}
