package api

import (
	"context"

	memorycatalogcommand "myai/core/application/memory/catalog/command"
)

// UsageRecorder is the narrow catalog capability needed after a retrieval hit.
type UsageRecorder interface {
	RecordUse(ctx context.Context, command memorycatalogcommand.RecordUse) error
}
