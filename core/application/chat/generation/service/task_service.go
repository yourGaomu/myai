package service

import (
	"context"
	"errors"
	"strings"

	agentrunapi "myai/core/application/agentrun/api"
	agentruncommand "myai/core/application/agentrun/command"
	agentrunruntime "myai/core/application/agentrun/runtime"
	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
	domainagentrun "myai/core/domain/agentrun"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type TaskService struct {
	// TaskService creates the request-scoped checkpoint and Agent run before delegating generation.
	RequestIDs   generationport.RequestIDGenerator
	Recorders    generationport.TaskRecorderFactory
	Generator    generationapi.Generator
	Runs         agentrunapi.CommandService
	OnSaveError  func(error)
	OnCloseError func(error)
	OnRunError   func(error)
}

var _ generationapi.TaskService = TaskService{}

func (s TaskService) Generate(ctx context.Context, command generationcommand.GenerationTask) (response generationresult.GenerationResponse, resultErr error) {
	if command.Session == nil {
		return generationresult.GenerationResponse{}, errors.New("session is nil")
	}
	if s.RequestIDs == nil {
		return generationresult.GenerationResponse{}, errors.New("request id generator is nil")
	}
	if s.Generator == nil {
		return generationresult.GenerationResponse{}, errors.New("generation handler is nil")
	}

	requestID := s.RequestIDs.NewRequestID()
	runID := agentrunruntime.RunID(ctx)
	ownsRun := false
	if runID == "" && s.Runs != nil {
		metadata := agentrunruntime.MetadataFrom(ctx)
		run, err := s.Runs.Start(ctx, agentruncommand.Start{
			RequestID:   command.Stream.CorrelationID,
			SessionID:   command.Session.ID,
			ParentRunID: metadata.ParentRunID,
			PlanID:      metadata.PlanID,
			StepID:      metadata.StepID,
			TaskID:      metadata.TaskID,
			Kind:        runKind(command.Reason, command.Session, command.ForceChatMode),
			Title:       runTitle(command.Title, command.Reason),
			Reason:      command.Reason,
		})
		if err != nil {
			s.reportRunError(err)
		} else {
			runID = run.ID
			ownsRun = true
			ctx = agentrunruntime.WithRunID(ctx, runID)
			metadata := agentrunruntime.MetadataFrom(ctx)
			metadata.RunID = runID
			ctx = agentrunruntime.WithMetadata(ctx, metadata)
			if command.Stream.OnRunStarted != nil {
				command.Stream.OnRunStarted(run)
			}
		}
	}
	if ownsRun {
		defer func() {
			status := domainagentrun.StatusSucceeded
			errorMessage := ""
			if resultErr != nil {
				errorMessage = resultErr.Error()
				status = domainagentrun.StatusFailed
				if errors.Is(resultErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
					status = domainagentrun.StatusPaused
				}
			}
			finished, err := s.Runs.Finish(context.WithoutCancel(ctx), agentruncommand.Finish{
				RunID: runID, Status: status, ErrorMessage: errorMessage,
			})
			if err != nil {
				s.reportRunError(err)
				return
			}
			if command.Stream.OnRunCompleted != nil {
				command.Stream.OnRunCompleted(finished)
			}
		}()
	}

	stream := command.Stream
	if runID != "" && s.Runs != nil {
		runStream := newRunStreamRecorder(ctx, runID, s.Runs, command.Stream, s.reportRunError)
		defer runStream.Close()
		stream = runStream.Handler()
	}

	recorder := s.newRecorder(generationcommand.TaskRecord{
		Title: command.Title, Reason: command.Reason, SessionID: command.Session.ID, RequestID: requestID,
	})
	if recorder != nil {
		defer s.closeRecorder(recorder)
		defer s.saveRecorder(recorder)
		ctx = recorder.Attach(ctx)
	}
	response, resultErr = s.Generator.Generate(ctx, generationcommand.AssistantGeneration{
		Session: command.Session, LatestInput: command.LatestInput, RequestID: requestID,
		Stream: stream, CapturePlan: command.CapturePlan, ForceChatMode: command.ForceChatMode, Internal: command.Internal,
	})
	response.RunID = runID
	if resultErr == nil {
		s.recordCapturedPlan(context.WithoutCancel(ctx), runID, response.Plan, command.Reason, command.Stream)
	}
	return response, resultErr
}

func runKind(reason string, current *session.Session, forceChatMode bool) domainagentrun.Kind {
	switch strings.TrimSpace(reason) {
	case "regenerate response":
		return domainagentrun.KindRegenerate
	case "execute plan step":
		return domainagentrun.KindPlan
	case "autonomous planning":
		return domainagentrun.KindPlan
	case "user request":
		if !forceChatMode && current != nil && session.NormalizeAgentMode(current.AgentMode) == session.AgentModePlan {
			return domainagentrun.KindPlan
		}
		return domainagentrun.KindChat
	default:
		return domainagentrun.KindInternal
	}
}

func (s TaskService) recordCapturedPlan(ctx context.Context, runID string, currentPlan *agentplan.Plan, reason string, stream modelport.ChatStreamHandler) {
	if currentPlan == nil {
		return
	}
	if stream.OnPlanUpdate != nil {
		stream.OnPlanUpdate(agentplan.Clone(currentPlan))
	}
	if strings.TrimSpace(runID) == "" || s.Runs == nil {
		return
	}
	currentStep := 0
	title := "Plan ready for review"
	if strings.TrimSpace(reason) == "autonomous planning" {
		title = "Plan ready for execution"
	}
	if currentPlan.Status == agentplan.StatusDone {
		currentStep = len(currentPlan.Steps)
		title = "Plan completed"
	}
	event, err := s.Runs.Append(ctx, agentruncommand.Append{
		RunID: runID, Type: domainagentrun.EventTypePlanUpdate, Title: title,
		Content: currentPlan.Goal, Status: currentPlan.Status,
		CurrentStep: currentStep, TotalSteps: len(currentPlan.Steps),
	})
	if err != nil {
		s.reportRunError(err)
		return
	}
	if stream.OnRunEvent != nil {
		stream.OnRunEvent(event)
	}
}

func runTitle(title string, reason string) string {
	if title = strings.TrimSpace(title); title != "" {
		return title
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		return reason
	}
	return "Agent run"
}

func (s TaskService) newRecorder(record generationcommand.TaskRecord) generationport.TaskRecorder {
	if s.Recorders == nil {
		return nil
	}
	return s.Recorders.NewTaskRecorder(record)
}

func (s TaskService) saveRecorder(recorder generationport.TaskRecorder) {
	if err := recorder.Save(context.Background()); err != nil && s.OnSaveError != nil {
		s.OnSaveError(err)
	}
}

func (s TaskService) closeRecorder(recorder generationport.TaskRecorder) {
	if err := recorder.Close(); err != nil && s.OnCloseError != nil {
		s.OnCloseError(err)
	}
}

func (s TaskService) reportRunError(err error) {
	if err != nil && s.OnRunError != nil {
		s.OnRunError(err)
	}
}
