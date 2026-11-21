package service

import (
	"context"
	"errors"
	"time"

	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
	planport "myai/core/application/chat/plan/port"
	plancommand "myai/core/application/plan/command"
	planservice "myai/core/application/plan/service"
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	"myai/core/session"
)

type ResponseCommitService struct {
	// ResponseCommitService 负责把一次成功生成转换为会话状态，并在 Plan 模式下捕获结构化计划。
	Memory       generationport.ResponseMemoryStore
	PlanCapturer generationport.PlanCapturer
	PlanStates   planport.StateStore
	Now          func() time.Time
}

var _ generationapi.ResponseCommitter = ResponseCommitService{}

func (s ResponseCommitService) Commit(command generationcommand.Commit) (generationresult.Commit, error) {
	if command.Session == nil {
		return generationresult.Commit{}, errors.New("session is nil")
	}
	if s.Memory == nil {
		return generationresult.Commit{}, errors.New("session memory is nil")
	}
	var committed *domainmessage.Message
	if !command.Internal {
		// 1. 提交最终采样原文，避免把多轮汇总的 reasoning 写回单条历史。
		message := domainmessage.Assistant(command.Result.Content, command.Result.Reasoning, command.Result.ToolCalls)
		if command.Result.Message != nil {
			message = domainmessage.Clone(*command.Result.Message)
		}
		if message.Role != domainmessage.RoleAssistant {
			return generationresult.Commit{}, errors.New("expected assistant message")
		}
		usage := command.Result.Usage
		message.AssistantUsage = domainmessage.AssistantUsage{
			PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
			TotalTokens: usage.TotalTokens, ReasoningTokens: usage.ReasoningTokens,
			Available: usage.Available, PromptCachedTokens: usage.PromptCachedTokens,
			PromptCacheReported: usage.PromptCacheReported, PromptCacheIncomplete: usage.PromptCacheIncomplete,
		}
		_, appended, err := s.Memory.AppendMessagesTo(command.Session.ID, message)
		if err != nil {
			return generationresult.Commit{}, err
		}
		if len(appended) == 1 {
			committed = &appended[0]
		}
	}
	if err := s.Memory.AddUsageTo(command.Session.ID, command.Result.Usage); err != nil {
		return generationresult.Commit{}, err
	}
	var captured *agentplan.Plan
	var planErr error
	// Internal planning returns a draft to its caller; it must not replace the
	// live plan being recovered by the executor.
	if command.CapturePlan && session.NormalizeAgentMode(command.Session.AgentMode) == session.AgentModePlan {
		captured = s.planCapturer().Capture(command.Session.ID, command.LatestInput, command.Result.Content, s.now())
		if !command.Internal {
			if s.PlanStates != nil {
				state := plancommand.SaveState{SessionID: command.Session.ID, Model: command.Session.Model, Plan: captured, CheckExpected: true}
				if previous := command.Session.CurrentPlan; previous != nil {
					state.ExpectedPlanID, state.ExpectedRevision = previous.ID, previous.Revision
				}
				captured, planErr = s.PlanStates.Save(context.Background(), state)
				if planErr != nil {
					captured = nil
				}
			} else if err := s.Memory.SetCurrentPlanForSession(command.Session.ID, captured); err != nil {
				planErr, captured = err, nil
			}
		}
	}
	snapshot, err := s.Memory.Snapshot(command.Session.ID)
	if err != nil {
		return generationresult.Commit{}, err
	}
	return generationresult.Commit{Plan: captured, Session: snapshot, AssistantMessage: committed}, planErr
}

func (s ResponseCommitService) planCapturer() generationport.PlanCapturer {
	if s.PlanCapturer != nil {
		return s.PlanCapturer
	}
	return planservice.CaptureService{}
}

func (s ResponseCommitService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
