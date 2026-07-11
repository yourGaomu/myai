package api

import (
	"context"

	skillquery "myai/core/application/skill/query"
	skillresult "myai/core/application/skill/result"
)

type CatalogService interface {
	List(ctx context.Context, query skillquery.ListSkills) (skillresult.ListSkills, error)
	Root() string
}
