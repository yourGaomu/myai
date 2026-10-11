package api

import (
	"context"
	subagentquery "myai/core/application/subagent/query"
	subagentresult "myai/core/application/subagent/result"
)

type InspectionService interface {
	InspectTask(ctx context.Context, query subagentquery.InspectTask) (subagentresult.Inspection, error)
	ListTaskEvents(ctx context.Context, query subagentquery.TaskEvents) (subagentresult.EventPage, error)
}
