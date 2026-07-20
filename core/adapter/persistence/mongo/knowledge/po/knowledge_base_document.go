package po

import "time"

type KnowledgeBaseDocument struct {
	ID                    string     `bson:"_id"`
	CategoryID            string     `bson:"category_id,omitempty"`
	Name                  string     `bson:"name"`
	Description           string     `bson:"description,omitempty"`
	RAGEnabled            bool       `bson:"rag_enabled"`
	ActiveIndexProfileID  string     `bson:"active_index_profile_id,omitempty"`
	PendingIndexProfileID string     `bson:"pending_index_profile_id,omitempty"`
	Deleted               bool       `bson:"deleted"`
	DeletedAt             *time.Time `bson:"deleted_at,omitempty"`
	DeleteReason          string     `bson:"delete_reason,omitempty"`
	SyncSequence          int64      `bson:"sync_sequence"`
	CreatedAt             time.Time  `bson:"created_at"`
	UpdatedAt             time.Time  `bson:"updated_at"`
}
