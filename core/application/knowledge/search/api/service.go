package api

import (
	"context"

	searchcommand "myai/core/application/knowledge/search/command"
	searchresult "myai/core/application/knowledge/search/result"
)

type Service interface {
	Search(ctx context.Context, command searchcommand.Search) (searchresult.Search, error)
}
