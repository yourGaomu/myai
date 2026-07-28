package workspace

import "time"

type ChangeSetStatus string

const (
	ChangeSetStatusNone      ChangeSetStatus = "none"
	ChangeSetStatusPending   ChangeSetStatus = "pending"
	ChangeSetStatusApplied   ChangeSetStatus = "applied"
	ChangeSetStatusDiscarded ChangeSetStatus = "discarded"
	ChangeSetStatusConflict  ChangeSetStatus = "conflict"
)

type FileChangeType string

const (
	FileChangeAdded    FileChangeType = "added"
	FileChangeModified FileChangeType = "modified"
	FileChangeDeleted  FileChangeType = "deleted"
)

type FileChange struct {
	Path       string
	ChangeType FileChangeType
	BeforeHash string
	AfterHash  string
	BeforeSize int64
	AfterSize  int64
}

type ChangeSet struct {
	WorkspaceID  string
	Status       ChangeSetStatus
	Files        []FileChange
	CheckpointID string
	Message      string
	CreatedAt    time.Time
	AppliedAt    *time.Time
	DiscardedAt  *time.Time
}

func CloneChangeSet(source ChangeSet) ChangeSet {
	source.Files = append([]FileChange(nil), source.Files...)
	source.AppliedAt = cloneTimePointer(source.AppliedAt)
	source.DiscardedAt = cloneTimePointer(source.DiscardedAt)
	return source
}

func cloneTimePointer(source *time.Time) *time.Time {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}
