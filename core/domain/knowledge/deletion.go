package knowledge

import (
	"fmt"
	"time"
)

type Deletion struct {
	Deleted      bool
	DeletedAt    *time.Time
	DeleteReason string
}

func (d Deletion) Validate() error {
	if d.Deleted && d.DeletedAt == nil {
		return fmt.Errorf("deleted_at is required when deleted is true")
	}
	if !d.Deleted && d.DeletedAt != nil {
		return fmt.Errorf("deleted_at must be nil when deleted is false")
	}
	return nil
}
