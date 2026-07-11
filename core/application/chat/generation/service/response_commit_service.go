package service

import (
	"errors"
	"time"

	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
	planservice "myai/core/application/plan/service"
	agentplan "myai/core/plan"
	"myai/core/session"
)

type ResponseCommitService struct {
	// ResponseCommitService 负责把一次成功生成转换为会话状态，并在 Plan 模式下捕获结构化计划。
	Memory       generationport.ResponseMemoryStore
	PlanCapturer generationport.PlanCapturer
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
	if err := s.Memory.AddAssistantMessageTo(command.Session.ID, command.Result.Content); err != nil {
		return generationresult.Commit{}, err
	}
	if err := s.Memory.AddUsageTo(command.Session.ID, command.Result.Usage); err != nil {
		return generationresult.Commit{}, err
	}
	// Chat 模式只保存回答；Plan 模式才解析 Markdown Plan，避免普通回复被误识别为计划。
	if !command.CapturePlan || session.NormalizeAgentMode(command.Session.AgentMode) != session.AgentModePlan {
		return generationresult.Commit{}, nil
	}
	currentPlan := s.planCapturer().Capture(command.Session.ID, command.LatestInput, command.Result.Content, s.now())
	command.Session.CurrentPlan = agentplan.Clone(currentPlan)
	if err := s.Memory.SetCurrentPlanForSession(command.Session.ID, currentPlan); err != nil {
		return generationresult.Commit{}, err
	}
	return generationresult.Commit{Plan: currentPlan}, nil
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
