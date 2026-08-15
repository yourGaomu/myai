package po

import "time"

type ScopeDocument struct {
	Type string `bson:"type"`
	Key  string `bson:"key,omitempty"`
}

type ContentDocument struct {
	Goal              string `bson:"goal"`
	ApplicableContext string `bson:"applicable_context,omitempty"`
	Approach          string `bson:"approach,omitempty"`
	Result            string `bson:"result,omitempty"`
	PainPoints        string `bson:"pain_points,omitempty"`
	RootCause         string `bson:"root_cause,omitempty"`
	Lessons           string `bson:"lessons,omitempty"`
	Verification      string `bson:"verification,omitempty"`
}

type SourceDocument struct {
	Type       string    `bson:"type"`
	SessionID  string    `bson:"session_id,omitempty"`
	AgentRunID string    `bson:"agent_run_id,omitempty"`
	EventIDs   []string  `bson:"event_ids,omitempty"`
	CreatedAt  time.Time `bson:"created_at"`
}

type RevisionDocument struct {
	ID         string           `bson:"id"`
	Version    int              `bson:"version"`
	Content    ContentDocument  `bson:"content"`
	Confidence float64          `bson:"confidence"`
	Author     string           `bson:"author"`
	Sources    []SourceDocument `bson:"sources,omitempty"`
	CreatedAt  time.Time        `bson:"created_at"`
}

type MemoryDocument struct {
	ID             string             `bson:"_id"`
	Title          string             `bson:"title"`
	Kind           string             `bson:"kind"`
	Scope          ScopeDocument      `bson:"scope"`
	Tags           []string           `bson:"tags,omitempty"`
	Status         string             `bson:"status"`
	CurrentVersion int                `bson:"current_version"`
	Revisions      []RevisionDocument `bson:"revisions"`
	HumanLocked    bool               `bson:"human_locked,omitempty"`
	SupersedesID   string             `bson:"supersedes_id,omitempty"`
	UseCount       int64              `bson:"use_count,omitempty"`
	LastUsedAt     *time.Time         `bson:"last_used_at,omitempty"`
	DeletedAt      *time.Time         `bson:"deleted_at,omitempty"`
	DeletionReason string             `bson:"deletion_reason,omitempty"`
	CreatedAt      time.Time          `bson:"created_at"`
	UpdatedAt      time.Time          `bson:"updated_at"`
}

type CandidateDocument struct {
	ID             string           `bson:"_id"`
	OriginKey      string           `bson:"origin_key,omitempty"`
	Title          string           `bson:"title"`
	Kind           string           `bson:"kind"`
	Scope          ScopeDocument    `bson:"scope"`
	Tags           []string         `bson:"tags,omitempty"`
	Content        ContentDocument  `bson:"content"`
	Confidence     float64          `bson:"confidence"`
	Sources        []SourceDocument `bson:"sources,omitempty"`
	Status         string           `bson:"status"`
	TargetMemoryID string           `bson:"target_memory_id,omitempty"`
	ReviewNote     string           `bson:"review_note,omitempty"`
	CreatedAt      time.Time        `bson:"created_at"`
	UpdatedAt      time.Time        `bson:"updated_at"`
}

type ExtractionJobDocument struct {
	ID               string     `bson:"_id"`
	AgentRunID       string     `bson:"agent_run_id"`
	ExtractorVersion string     `bson:"extractor_version"`
	Status           string     `bson:"status"`
	Attempts         int        `bson:"attempts,omitempty"`
	LastError        string     `bson:"last_error,omitempty"`
	CreatedAt        time.Time  `bson:"created_at"`
	UpdatedAt        time.Time  `bson:"updated_at"`
	CompletedAt      *time.Time `bson:"completed_at,omitempty"`
}

type DreamActionDocument struct {
	CandidateID   string `bson:"candidate_id,omitempty"`
	MemoryID      string `bson:"memory_id,omitempty"`
	Decision      string `bson:"decision"`
	Reason        string `bson:"reason,omitempty"`
	Applied       bool   `bson:"applied,omitempty"`
	FailureReason string `bson:"failure_reason,omitempty"`
}

type DreamRunDocument struct {
	ID              string                `bson:"_id"`
	Status          string                `bson:"status"`
	Trigger         string                `bson:"trigger,omitempty"`
	CandidateCount  int                   `bson:"candidate_count,omitempty"`
	CreatedCount    int                   `bson:"created_count,omitempty"`
	MergedCount     int                   `bson:"merged_count,omitempty"`
	SupersededCount int                   `bson:"superseded_count,omitempty"`
	RejectedCount   int                   `bson:"rejected_count,omitempty"`
	Actions         []DreamActionDocument `bson:"actions,omitempty"`
	LastError       string                `bson:"last_error,omitempty"`
	StartedAt       time.Time             `bson:"started_at"`
	FinishedAt      *time.Time            `bson:"finished_at,omitempty"`
}
