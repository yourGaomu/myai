package workspace

import (
	"time"

	domainworkspace "myai/core/domain/workspace"
)

type PrepareRequest struct {
	WorkspaceID string
	Mode        domainworkspace.IsolationMode
	SourceRoot  string
	TaskID      string
	SessionID   string
}

type PreparedWorkspace struct {
	Reference domainworkspace.Reference
}

type CollectRequest struct {
	Reference domainworkspace.Reference
}

type CollectedChanges struct {
	ChangeSet domainworkspace.ChangeSet
}

type ApplyRequest struct {
	Reference domainworkspace.Reference
	TaskID    string
	SessionID string
	RequestID string
	Title     string
}

type AppliedChanges struct {
	ChangeSet domainworkspace.ChangeSet
}

type DiscardRequest struct {
	Reference   domainworkspace.Reference
	DiscardedAt time.Time
}

type DiscardedChanges struct {
	ChangeSet domainworkspace.ChangeSet
}
