package po

import "time"

type ChunkDocument struct {
	ID                  string     `bson:"_id"`
	KnowledgeBaseID     string     `bson:"knowledge_base_id"`
	DocumentID          string     `bson:"document_id"`
	DocumentVersion     int64      `bson:"document_version"`
	ParsingProfileID    string     `bson:"parsing_profile_id"`
	ChunkingProfileID   string     `bson:"chunking_profile_id"`
	Ordinal             int        `bson:"ordinal"`
	Text                string     `bson:"text"`
	ContentHash         string     `bson:"content_hash"`
	StartOffset         int        `bson:"start_offset"`
	EndOffset           int        `bson:"end_offset"`
	SourcePage          int        `bson:"source_page,omitempty"`
	SourceHeading       string     `bson:"source_heading,omitempty"`
	EmbeddingProfileIDs []string   `bson:"embedding_profile_ids,omitempty"`
	Deleted             bool       `bson:"deleted"`
	DeletedAt           *time.Time `bson:"deleted_at,omitempty"`
	DeleteReason        string     `bson:"delete_reason,omitempty"`
	SyncSequence        int64      `bson:"sync_sequence"`
	CreatedAt           time.Time  `bson:"created_at"`
	UpdatedAt           time.Time  `bson:"updated_at"`
}
