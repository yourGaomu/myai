package po

import "time"

type DefinitionDocument struct {
	ID             string     `bson:"_id"`
	Name           string     `bson:"name"`
	Description    string     `bson:"description,omitempty"`
	SystemPrompt   string     `bson:"system_prompt"`
	ModelID        string     `bson:"model_id,omitempty"`
	AllowedTools   []string   `bson:"allowed_tools,omitempty"`
	CapabilityMode string     `bson:"capability_mode"`
	IsolationMode  string     `bson:"isolation_mode"`
	MaxTurns       int        `bson:"max_turns"`
	TimeoutSeconds int        `bson:"timeout_seconds"`
	Enabled        bool       `bson:"enabled"`
	Version        int64      `bson:"version"`
	Source         string     `bson:"source"`
	Deleted        bool       `bson:"deleted"`
	DeletedAt      *time.Time `bson:"deleted_at,omitempty"`
	CreatedAt      time.Time  `bson:"created_at"`
	UpdatedAt      time.Time  `bson:"updated_at"`
}

type DefinitionSnapshotDocument struct {
	ID             string   `bson:"id"`
	Name           string   `bson:"name"`
	SystemPrompt   string   `bson:"system_prompt"`
	ModelID        string   `bson:"model_id,omitempty"`
	AllowedTools   []string `bson:"allowed_tools,omitempty"`
	CapabilityMode string   `bson:"capability_mode"`
	IsolationMode  string   `bson:"isolation_mode"`
	MaxTurns       int      `bson:"max_turns"`
	TimeoutSeconds int      `bson:"timeout_seconds"`
	Version        int64    `bson:"version"`
}

type WorkspaceReferenceDocument struct {
	ID        string `bson:"id,omitempty"`
	Mode      string `bson:"mode,omitempty"`
	Root      string `bson:"root,omitempty"`
	SandboxID string `bson:"sandbox_id,omitempty"`
}

type FileChangeDocument struct {
	Path       string `bson:"path"`
	ChangeType string `bson:"change_type"`
	BeforeHash string `bson:"before_hash,omitempty"`
	AfterHash  string `bson:"after_hash,omitempty"`
	BeforeSize int64  `bson:"before_size,omitempty"`
	AfterSize  int64  `bson:"after_size,omitempty"`
}

type ChangeSetDocument struct {
	WorkspaceID  string               `bson:"workspace_id,omitempty"`
	Status       string               `bson:"status,omitempty"`
	Files        []FileChangeDocument `bson:"files,omitempty"`
	CheckpointID string               `bson:"checkpoint_id,omitempty"`
	Message      string               `bson:"message,omitempty"`
	CreatedAt    time.Time            `bson:"created_at,omitempty"`
	AppliedAt    *time.Time           `bson:"applied_at,omitempty"`
	DiscardedAt  *time.Time           `bson:"discarded_at,omitempty"`
}

type TaskDocument struct {
	ID                string                     `bson:"_id"`
	ParentSessionID   string                     `bson:"parent_session_id"`
	ParentTaskID      string                     `bson:"parent_task_id,omitempty"`
	ParentRunID       string                     `bson:"parent_run_id,omitempty"`
	PlanID            string                     `bson:"plan_id,omitempty"`
	StepID            string                     `bson:"step_id,omitempty"`
	ChildSessionID    string                     `bson:"child_session_id"`
	AgentPath         string                     `bson:"agent_path,omitempty"`
	AgentNickname     string                     `bson:"agent_nickname,omitempty"`
	CreatedRequestID  string                     `bson:"created_request_id,omitempty"`
	DefinitionID      string                     `bson:"definition_id"`
	DefinitionVersion int64                      `bson:"definition_version"`
	Definition        DefinitionSnapshotDocument `bson:"definition"`
	Instruction       string                     `bson:"instruction"`
	Title             string                     `bson:"title"`
	Status            string                     `bson:"status"`
	CurrentRunID      string                     `bson:"current_run_id"`
	Workspace         WorkspaceReferenceDocument `bson:"workspace"`
	ChangeSet         ChangeSetDocument          `bson:"change_set,omitempty"`
	Result            string                     `bson:"result,omitempty"`
	Reasoning         string                     `bson:"reasoning,omitempty"`
	ErrorMessage      string                     `bson:"error_message,omitempty"`
	Unread            bool                       `bson:"unread"`
	CreatedAt         time.Time                  `bson:"created_at"`
	UpdatedAt         time.Time                  `bson:"updated_at"`
	StartedAt         *time.Time                 `bson:"started_at,omitempty"`
	CompletedAt       *time.Time                 `bson:"completed_at,omitempty"`
}

type RunDocument struct {
	ID           string     `bson:"_id"`
	TaskID       string     `bson:"task_id"`
	Sequence     int        `bson:"sequence"`
	Instruction  string     `bson:"instruction"`
	Status       string     `bson:"status"`
	Result       string     `bson:"result,omitempty"`
	ErrorMessage string     `bson:"error_message,omitempty"`
	CreatedAt    time.Time  `bson:"created_at"`
	StartedAt    *time.Time `bson:"started_at,omitempty"`
	CompletedAt  *time.Time `bson:"completed_at,omitempty"`
}

// TaskEventDocument is an append-only replay record. The task snapshot keeps
// reconnecting clients useful even when they missed intermediate state
// updates; runtime fields carry reasoning/tool deltas for live rendering.
type TaskEventDocument struct {
	ID           string       `bson:"_id"`
	Sequence     uint64       `bson:"sequence"`
	Kind         string       `bson:"kind"`
	Task         TaskDocument `bson:"task"`
	RunID        string       `bson:"run_id,omitempty"`
	Content      string       `bson:"content,omitempty"`
	ToolName     string       `bson:"tool_name,omitempty"`
	Arguments    string       `bson:"arguments,omitempty"`
	Status       string       `bson:"status,omitempty"`
	ErrorCode    string       `bson:"error_code,omitempty"`
	ErrorMessage string       `bson:"error_message,omitempty"`
	Truncated    bool         `bson:"truncated,omitempty"`
	Delta        bool         `bson:"delta,omitempty"`
	EmittedAt    time.Time    `bson:"emitted_at"`
}
