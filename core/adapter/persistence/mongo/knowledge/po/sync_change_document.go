package po

import "time"

type SyncChangeDocument struct {
	ID              string    `bson:"_id"`
	Sequence        int64     `bson:"sequence"`
	KnowledgeBaseID string    `bson:"knowledge_base_id"`
	EntityType      string    `bson:"entity_type"`
	EntityID        string    `bson:"entity_id"`
	Operation       string    `bson:"operation"`
	EntityVersion   int64     `bson:"entity_version"`
	Deleted         bool      `bson:"deleted"`
	OccurredAt      time.Time `bson:"occurred_at"`
}
