package chat

import (
	"context"
	"errors"

	subagentport "myai/core/port/subagent"
	"myai/core/service"
)

type Runner struct {
	Chat *service.ChatService
}

var _ subagentport.AgentRunner = Runner{}

func (runner Runner) Run(ctx context.Context, request subagentport.AgentRunRequest) (subagentport.AgentRunResult, error) {
	if runner.Chat == nil {
		return subagentport.AgentRunResult{}, errors.New("subagent chat runner is nil")
	}
	response, err := runner.Chat.SendMessageStreamForSession(ctx, request.SessionID, request.Instruction, request.Stream)
	if err != nil {
		return subagentport.AgentRunResult{}, err
	}
	return subagentport.AgentRunResult{
		Content: response.Result.Content, Reasoning: response.Result.Reasoning, Usage: response.Result.Usage,
	}, nil
}
