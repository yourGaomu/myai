package subagent

import "context"

type AgentRunner interface {
	Run(ctx context.Context, request AgentRunRequest) (AgentRunResult, error)
}
