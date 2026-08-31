package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentrunruntime "myai/core/application/agentrun/runtime"
	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	plancommand "myai/core/application/chat/plan/command"
	planport "myai/core/application/chat/plan/port"
	messagecommand "myai/core/application/session/message/command"
	domainmessage "myai/core/domain/message"
	plan "myai/core/plan"
	"myai/core/session"
)

// GenerationRecoveryPlanner asks the same model for a replacement plan after
// an execution step fails. It uses a plan-mode child turn, so recovery can
// inspect the workspace but cannot mutate it before execution resumes.
type GenerationRecoveryPlanner struct {
	Messages   planport.MessageAppender
	Generation generationapi.TaskService
}

var _ planport.RecoveryPlanner = GenerationRecoveryPlanner{}

func (p GenerationRecoveryPlanner) Recover(ctx context.Context, request plancommand.RecoveryRequest) (*plan.Plan, error) {
	if p.Messages == nil || p.Generation == nil {
		return nil, errors.New("plan recovery planner is not configured")
	}
	if request.Plan == nil {
		return nil, errors.New("plan recovery requires a plan")
	}
	input := fmt.Sprintf("A plan execution step failed and needs recovery.\n\nFailed step: %s\nError: %s\nAttempt: %d\n\nInspect the workspace with read-only tools if needed, then return a concise Plan section containing only replacement or follow-up steps. Do not make changes in this recovery turn.", request.Step.Title, strings.TrimSpace(request.Error), request.Attempt)
	prepared, err := p.Messages.AppendUserMessage(ctx, messagecommand.AppendUserMessage{
		SessionID: request.Plan.SessionID, Input: input, ForcePlanMode: true,
		SyntheticReason:      domainmessage.SyntheticReasonAutonomousPlanning,
		DeduplicateSynthetic: true,
	})
	if err != nil {
		return nil, err
	}
	planningSession := session.Clone(prepared.Session)
	if planningSession == nil {
		return nil, errors.New("plan recovery session is nil")
	}
	planningSession.AgentMode = session.AgentModePlan
	metadata := agentrunruntime.MetadataFrom(ctx)
	metadata.ParentRunID = agentrunruntime.RunID(ctx)
	metadata.RunID = ""
	ctx = agentrunruntime.WithMetadata(ctx, metadata)
	ctx = agentrunruntime.WithRunID(ctx, "")
	response, err := p.Generation.Generate(ctx, generationcommand.GenerationTask{
		Session: planningSession, LatestInput: input, Reason: "recover plan", CapturePlan: true,
		Internal: true,
	})
	if err != nil {
		return nil, err
	}
	if response.Plan == nil || len(response.Plan.Steps) == 0 {
		return nil, errors.New("plan recovery did not produce replacement steps")
	}
	return response.Plan, nil
}
