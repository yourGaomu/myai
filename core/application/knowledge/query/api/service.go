package api

import (
	"context"

	querycommand "myai/core/application/knowledge/query/command"
	queryresult "myai/core/application/knowledge/query/result"
)

type Service interface {
	Documents(ctx context.Context, command querycommand.Documents) (queryresult.Documents, error)
	IndexProfiles(ctx context.Context, command querycommand.IndexProfiles) (queryresult.IndexProfiles, error)
}
