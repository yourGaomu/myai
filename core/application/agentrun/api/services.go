package api

import (
	"context"

	agentruncommand "myai/core/application/agentrun/command"
	agentrunquery "myai/core/application/agentrun/query"
	agentrunresult "myai/core/application/agentrun/result"
	domainagentrun "myai/core/domain/agentrun"
)

type CommandService interface {
	Start(ctx context.Context, command agentruncommand.Start) (domainagentrun.Run, error)
	Append(ctx context.Context, command agentruncommand.Append) (domainagentrun.Event, error)
	ReplaceEventContent(ctx context.Context, command agentruncommand.ReplaceEventContent) error
	Finish(ctx context.Context, command agentruncommand.Finish) (domainagentrun.Run, error)
}

type QueryService interface {
	ListSessionRuns(ctx context.Context, query agentrunquery.ListSessionRuns) ([]agentrunresult.Snapshot, error)
}
