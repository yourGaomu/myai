package knowledge

import (
	"fmt"
	"strings"
	"time"
)

type IndexingStage string
type IndexingJobStatus string

const (
	IndexingStageUpload    IndexingStage = "upload"
	IndexingStageParse     IndexingStage = "parse"
	IndexingStageChunk     IndexingStage = "chunk"
	IndexingStageEmbed     IndexingStage = "embed"
	IndexingStageIndex     IndexingStage = "index"
	IndexingStageCompleted IndexingStage = "completed"

	IndexingJobStatusPending   IndexingJobStatus = "pending"
	IndexingJobStatusRunning   IndexingJobStatus = "running"
	IndexingJobStatusCompleted IndexingJobStatus = "completed"
	IndexingJobStatusFailed    IndexingJobStatus = "failed"
)

type IndexingJob struct {
	ID              string
	KnowledgeBaseID string
	DocumentID      string
	IndexProfileID  string
	Stage           IndexingStage
	Status          IndexingJobStatus
	TotalChunks     int
	CompletedChunks int
	FailedChunks    int
	LastError       string
	RetryCount      int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
}

func (job IndexingJob) Validate() error {
	if strings.TrimSpace(job.ID) == "" {
		return fmt.Errorf("indexing job id is required")
	}
	if strings.TrimSpace(job.KnowledgeBaseID) == "" {
		return fmt.Errorf("knowledge base id is required")
	}
	if strings.TrimSpace(job.DocumentID) == "" {
		return fmt.Errorf("document id is required")
	}
	if strings.TrimSpace(job.IndexProfileID) == "" {
		return fmt.Errorf("index profile id is required")
	}
	if !IsIndexingStage(job.Stage) {
		return fmt.Errorf("unsupported indexing stage %q", job.Stage)
	}
	if !IsIndexingJobStatus(job.Status) {
		return fmt.Errorf("unsupported indexing job status %q", job.Status)
	}
	if job.TotalChunks < 0 || job.CompletedChunks < 0 || job.FailedChunks < 0 {
		return fmt.Errorf("indexing chunk counters must not be negative")
	}
	if job.CompletedChunks+job.FailedChunks > job.TotalChunks {
		return fmt.Errorf("completed and failed chunks exceed total chunks")
	}
	if job.RetryCount < 0 {
		return fmt.Errorf("retry count must not be negative")
	}
	if job.Status == IndexingJobStatusFailed && strings.TrimSpace(job.LastError) == "" {
		return fmt.Errorf("last error is required for failed indexing job")
	}
	if job.Status == IndexingJobStatusCompleted && job.CompletedAt == nil {
		return fmt.Errorf("completed_at is required for completed indexing job")
	}
	return nil
}

func IsIndexingStage(stage IndexingStage) bool {
	switch stage {
	case IndexingStageUpload,
		IndexingStageParse,
		IndexingStageChunk,
		IndexingStageEmbed,
		IndexingStageIndex,
		IndexingStageCompleted:
		return true
	default:
		return false
	}
}

func IsIndexingJobStatus(status IndexingJobStatus) bool {
	switch status {
	case IndexingJobStatusPending,
		IndexingJobStatusRunning,
		IndexingJobStatusCompleted,
		IndexingJobStatusFailed:
		return true
	default:
		return false
	}
}
