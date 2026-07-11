package generation

import (
	"context"

	sessioncommand "myai/core/application/session/command"
)

type SummaryPersistence interface {
	Save(ctx context.Context, command sessioncommand.SaveSession) error
}
