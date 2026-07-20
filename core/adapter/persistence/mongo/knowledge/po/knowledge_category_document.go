package po

import "time"

type KnowledgeCategoryDocument struct {
	ID           string     `bson:"_id"`
	Name         string     `bson:"name"`
	ParentID     string     `bson:"parent_id,omitempty"`
	AncestorIDs  []string   `bson:"ancestor_ids,omitempty"`
	SortOrder    int        `bson:"sort_order,omitempty"`
	Deleted      bool       `bson:"deleted"`
	DeletedAt    *time.Time `bson:"deleted_at,omitempty"`
	DeleteReason string     `bson:"delete_reason,omitempty"`
	CreatedAt    time.Time  `bson:"created_at"`
	UpdatedAt    time.Time  `bson:"updated_at"`
}
