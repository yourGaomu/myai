package po

import "time"

type KnowledgeDocument struct {
	ID              string     `bson:"_id"`
	KnowledgeBaseID string     `bson:"knowledge_base_id"`
	FileName        string     `bson:"file_name"`
	ContentType     string     `bson:"content_type,omitempty"`
	ObjectKey       string     `bson:"object_key"`
	ContentHash     string     `bson:"content_hash"`
	Version         int64      `bson:"version"`
	Status          string     `bson:"status"`
	FailureReason   string     `bson:"failure_reason,omitempty"`
	Deleted         bool       `bson:"deleted"`
	DeletedAt       *time.Time `bson:"deleted_at,omitempty"`
	DeleteReason    string     `bson:"delete_reason,omitempty"`
	SyncSequence    int64      `bson:"sync_sequence"`
	CreatedAt       time.Time  `bson:"created_at"`
	UpdatedAt       time.Time  `bson:"updated_at"`
}
