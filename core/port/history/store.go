package history

import (
	"context"

	domainhistory "myai/core/domain/history"
)

type Store interface {
	Close() error
	HasBaseline(ctx context.Context, workspace string) (bool, error)
	LoadBaseline(ctx context.Context, workspace string) (map[string]domainhistory.FileSnapshot, error)
	ReplaceBaseline(ctx context.Context, workspace string, files map[string]domainhistory.FileSnapshot) error
	SaveCheckpoint(ctx context.Context, checkpoint domainhistory.Checkpoint, changes []domainhistory.FileChange) (string, error)
	ListCheckpoints(ctx context.Context, workspace string, limit int) ([]domainhistory.CheckpointSummary, error)
	LoadCheckpointChanges(ctx context.Context, workspace string, checkpointID string) ([]domainhistory.StoredFileChange, error)
}

type StoreFactory interface {
	Open(path string) (Store, error)
	DefaultPath(workspace string) (string, error)
}
