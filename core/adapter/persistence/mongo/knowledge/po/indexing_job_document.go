package po

import "time"

type IndexingJobDocument struct {
	ID              string     `bson:"_id"`
	KnowledgeBaseID string     `bson:"knowledge_base_id"`
	DocumentID      string     `bson:"document_id"`
	IndexProfileID  string     `bson:"index_profile_id"`
	Stage           string     `bson:"stage"`
	Status          string     `bson:"status"`
	TotalChunks     int        `bson:"total_chunks"`
	CompletedChunks int        `bson:"completed_chunks"`
	FailedChunks    int        `bson:"failed_chunks"`
	LastError       string     `bson:"last_error,omitempty"`
	RetryCount      int        `bson:"retry_count"`
	CreatedAt       time.Time  `bson:"created_at"`
	UpdatedAt       time.Time  `bson:"updated_at"`
	CompletedAt     *time.Time `bson:"completed_at,omitempty"`
}
