package port

import (
	"context"

	domainskill "myai/core/skill"
)

type Catalog interface {
	Reload(ctx context.Context) error
	List() []domainskill.Skill
	Root() string
}
