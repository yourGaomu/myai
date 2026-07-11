package history

import (
	"os"
	"time"
)

type FileSnapshot struct {
	Path      string
	Size      int64
	Hash      [32]byte
	Content   []byte
	Binary    bool
	TooLarge  bool
	Mode      os.FileMode
	Available bool
}

type Checkpoint struct {
	ID        string
	Workspace string
	SessionID string
	RequestID string
	Title     string
	Reason    string
	CreatedAt time.Time
}

type CheckpointSummary struct {
	ID          string
	Workspace   string
	SessionID   string
	RequestID   string
	Title       string
	Reason      string
	ChangeCount int
	CreatedAt   time.Time
}

type FileChange struct {
	Path       string
	ChangeType string
	Before     *FileSnapshot
	After      *FileSnapshot
	CreatedAt  time.Time
}

type StoredFileChange struct {
	ID           int64
	CheckpointID string
	Path         string
	ChangeType   string
	Before       *FileSnapshot
	After        *FileSnapshot
	CreatedAt    time.Time
}
