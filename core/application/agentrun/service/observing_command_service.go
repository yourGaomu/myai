package service

import (
	"context"
	"errors"

	agentrunapi "myai/core/application/agentrun/api"
	agentruncommand "myai/core/application/agentrun/command"
	domainagentrun "myai/core/domain/agentrun"
	agentrunport "myai/core/port/agentrun"
)

type ObservingCommandService struct {
	Inner    agentrunapi.CommandService
	Observer agentrunport.CompletionObserver
}

var _ agentrunapi.CommandService = ObservingCommandService{}

func (service ObservingCommandService) Start(ctx context.Context, command agentruncommand.Start) (domainagentrun.Run, error) {
	if service.Inner == nil {
		return domainagentrun.Run{}, errors.New("agent run command service is nil")
	}
	return service.Inner.Start(ctx, command)
}

func (service ObservingCommandService) Append(ctx context.Context, command agentruncommand.Append) (domainagentrun.Event, error) {
	if service.Inner == nil {
		return domainagentrun.Event{}, errors.New("agent run command service is nil")
	}
	return service.Inner.Append(ctx, command)
}

func (service ObservingCommandService) ReplaceEventContent(ctx context.Context, command agentruncommand.ReplaceEventContent) error {
	if service.Inner == nil {
		return errors.New("agent run command service is nil")
	}
	return service.Inner.ReplaceEventContent(ctx, command)
}

func (service ObservingCommandService) Finish(ctx context.Context, command agentruncommand.Finish) (domainagentrun.Run, error) {
	if service.Inner == nil {
		return domainagentrun.Run{}, errors.New("agent run command service is nil")
	}
	run, err := service.Inner.Finish(ctx, command)
	if err == nil && service.Observer != nil {
		service.Observer.AgentRunCompleted(context.WithoutCancel(ctx), run)
	}
	return run, err
}
