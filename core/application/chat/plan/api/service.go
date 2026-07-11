package api

import (
	"context"

	plancommand "myai/core/application/chat/plan/command"
	planport "myai/core/application/chat/plan/port"
	planresult "myai/core/application/chat/plan/result"
)

type Service interface {
	Execute(ctx context.Context, command plancommand.Execute, updates planport.UpdateSink) (planresult.Execution, error)
}
