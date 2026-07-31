package service

import (
	"context"
	"errors"
	"strings"
	"time"

	agentrunapi "myai/core/application/agentrun/api"
	agentruncommand "myai/core/application/agentrun/command"
	domainagentrun "myai/core/domain/agentrun"
	agentrunport "myai/core/port/agentrun"
)

type CommandService struct {
	Repository agentrunport.Repository
	IDs        agentrunport.IDGenerator
	Now        func() time.Time
}

var _ agentrunapi.CommandService = CommandService{}

func (s CommandService) Start(ctx context.Context, command agentruncommand.Start) (domainagentrun.Run, error) {
	if s.Repository == nil {
		return domainagentrun.Run{}, errors.New("agent run repository is nil")
	}
	if s.IDs == nil {
		return domainagentrun.Run{}, errors.New("agent run id generator is nil")
	}
	runID := strings.TrimSpace(command.RunID)
	if runID == "" {
		runID = s.IDs.NewID()
	}
	requestID := strings.TrimSpace(command.RequestID)
	if requestID == "" {
		requestID = runID
	}
	run := domainagentrun.Run{
		ID: runID, RequestID: requestID, SessionID: strings.TrimSpace(command.SessionID),
		Kind: command.Kind, Title: strings.TrimSpace(command.Title), Reason: strings.TrimSpace(command.Reason),
		Status: domainagentrun.StatusRunning, TotalSteps: command.TotalSteps, StartedAt: s.now(),
	}
	if run.Kind == "" {
		run.Kind = domainagentrun.KindChat
	}
	if err := run.Validate(); err != nil {
		return domainagentrun.Run{}, err
	}
	if err := s.Repository.SaveRun(ctx, run); err != nil {
		return domainagentrun.Run{}, err
	}
	return run, nil
}

func (s CommandService) Append(ctx context.Context, command agentruncommand.Append) (domainagentrun.Event, error) {
	if s.Repository == nil {
		return domainagentrun.Event{}, errors.New("agent run repository is nil")
	}
	if s.IDs == nil {
		return domainagentrun.Event{}, errors.New("agent run id generator is nil")
	}
	run, err := s.Repository.GetRun(ctx, strings.TrimSpace(command.RunID))
	if err != nil {
		return domainagentrun.Event{}, err
	}
	sequence, err := s.Repository.NextEventSequence(ctx, run.ID)
	if err != nil {
		return domainagentrun.Event{}, err
	}
	event := domainagentrun.Event{
		ID: s.IDs.NewID(), RunID: run.ID, SessionID: run.SessionID, Sequence: sequence,
		Type: command.Type, Title: strings.TrimSpace(command.Title), Content: command.Content,
		ToolName: command.ToolName, Arguments: command.Arguments, Status: command.Status,
		ErrorCode: command.ErrorCode, ErrorMessage: command.ErrorMessage, Truncated: command.Truncated,
		CurrentStep: command.CurrentStep, TotalSteps: command.TotalSteps, CreatedAt: s.now(),
	}
	if err := event.Validate(); err != nil {
		return domainagentrun.Event{}, err
	}
	if err := s.Repository.SaveEvent(ctx, event); err != nil {
		return domainagentrun.Event{}, err
	}
	return event, nil
}

func (s CommandService) ReplaceEventContent(ctx context.Context, command agentruncommand.ReplaceEventContent) error {
	if s.Repository == nil {
		return errors.New("agent run repository is nil")
	}
	if strings.TrimSpace(command.RunID) == "" || strings.TrimSpace(command.EventID) == "" {
		return errors.New("agent run event identity is empty")
	}
	return s.Repository.ReplaceEventContent(ctx, command.RunID, command.EventID, command.Content, command.Truncated)
}

func (s CommandService) Finish(ctx context.Context, command agentruncommand.Finish) (domainagentrun.Run, error) {
	if s.Repository == nil {
		return domainagentrun.Run{}, errors.New("agent run repository is nil")
	}
	run, err := s.Repository.GetRun(ctx, strings.TrimSpace(command.RunID))
	if err != nil {
		return domainagentrun.Run{}, err
	}
	if run.IsTerminal() {
		return run, nil
	}
	status := command.Status
	if status == "" {
		status = domainagentrun.StatusSucceeded
	}
	now := s.now()
	run.Status = status
	run.ErrorMessage = strings.TrimSpace(command.ErrorMessage)
	run.CurrentStep = command.CurrentStep
	if command.TotalSteps > 0 {
		run.TotalSteps = command.TotalSteps
	}
	run.FinishedAt = &now
	if err := run.Validate(); err != nil {
		return domainagentrun.Run{}, err
	}
	eventType, title := terminalEvent(status)
	terminal, err := s.Append(ctx, agentruncommand.Append{
		RunID: run.ID, Type: eventType, Title: title, Content: run.ErrorMessage,
		Status: string(status), CurrentStep: run.CurrentStep, TotalSteps: run.TotalSteps,
	})
	if err != nil {
		return domainagentrun.Run{}, err
	}
	run.LastSequence = terminal.Sequence
	if err := s.Repository.SaveRun(ctx, run); err != nil {
		return domainagentrun.Run{}, err
	}
	return run, nil
}

func terminalEvent(status domainagentrun.Status) (domainagentrun.EventType, string) {
	switch status {
	case domainagentrun.StatusPaused:
		return domainagentrun.EventTypePaused, "Run paused"
	case domainagentrun.StatusCanceled:
		return domainagentrun.EventTypeCanceled, "Run canceled"
	case domainagentrun.StatusFailed:
		return domainagentrun.EventTypeFailed, "Run failed"
	default:
		return domainagentrun.EventTypeCompleted, "Run completed"
	}
}

func (s CommandService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
