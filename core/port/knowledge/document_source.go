package knowledge

import (
	"context"

	domainknowledge "myai/core/domain/knowledge"
)

type DocumentSourceReader interface {
	Read(ctx context.Context, request domainknowledge.DocumentSourceRequest) (domainknowledge.DocumentSourceContent, error)
}
