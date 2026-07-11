package plan

import (
	"context"

	agentplan "myai/core/plan"
)

type StateRepository interface {
	SaveCurrentPlan(ctx context.Context, sessionID string, model string, currentPlan *agentplan.Plan) error
}
