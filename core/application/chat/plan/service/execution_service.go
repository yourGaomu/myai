package service

import (
	"context"
	"errors"
	"strings"

	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	planapi "myai/core/application/chat/plan/api"
	plancommand "myai/core/application/chat/plan/command"
	planport "myai/core/application/chat/plan/port"
	planresult "myai/core/application/chat/plan/result"
	chatport "myai/core/application/chat/port"
	plancommandapp "myai/core/application/plan/command"
	planserviceapp "myai/core/application/plan/service"
	messagecommand "myai/core/application/session/message/command"
	agentplan "myai/core/plan"
	"myai/core/session"
)

type ExecutionService struct {
	// ExecutionService 只执行已经保存在 Session.CurrentPlan 中且由用户批准的计划。
	Models       chatport.ModelProvider
	Sessions     planport.SessionLoader
	Messages     planport.MessageAppender
	Generation   generationapi.TaskService
	PlanStates   planport.StateStore
	UserMessages planport.UserMessagePersistence
	Events       planport.SessionEventPublisher
	State        planserviceapp.StateService
	Inputs       planserviceapp.ExecutionInputBuilder
	Responses    planserviceapp.ResponseCombiner
}

var _ planapi.Service = ExecutionService{}

func (s ExecutionService) Execute(ctx context.Context, command plancommand.Execute, updates planport.UpdateSink) (planresult.Execution, error) {
	if s.Models == nil {
		return planresult.Execution{}, errors.New("llm client is nil")
	}
	if s.Sessions == nil || s.Messages == nil {
		return planresult.Execution{}, errors.New("session manager is nil")
	}
	if s.Generation == nil {
		return planresult.Execution{}, errors.New("generation task service is nil")
	}

	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return planresult.Execution{}, errors.New("session id is empty")
	}
	current, err := s.Sessions.Load(ctx, sessionID)
	if err != nil {
		return planresult.Execution{}, err
	}
	currentPlan := agentplan.Clone(current.CurrentPlan)
	if currentPlan == nil {
		return planresult.Execution{}, errors.New("current session has no plan")
	}
	if len(currentPlan.Steps) == 0 {
		return planresult.Execution{}, errors.New("current plan has no steps")
	}

	// 先把整体计划置为 running 并持久化，手机端会立即收到状态更新。
	currentPlan = s.State.Start(currentPlan)
	if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
		return planresult.Execution{}, err
	}

	combined := planresult.Execution{SessionID: current.ID}
	for index := range currentPlan.Steps {
		// 每个步骤都是独立生成任务；中途取消时保留已完成步骤，并把整体计划标记为 canceled。
		if err := ctx.Err(); err != nil {
			currentPlan = s.State.MarkCanceled(currentPlan)
			_, _ = s.savePlanState(context.Background(), current, currentPlan, updates)
			return planresult.Execution{}, err
		}

		currentPlan = s.State.MarkStepRunning(currentPlan, index)
		if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
			return planresult.Execution{}, err
		}

		// 把目标、当前步骤和完整计划转换成一条明确的用户消息，保证模型只处理当前步骤。
		input := s.Inputs.BuildStepInput(currentPlan, currentPlan.Steps[index], index, len(currentPlan.Steps))
		prepared, err := s.Messages.AppendUserMessage(ctx, messagecommand.AppendUserMessage{SessionID: current.ID, Input: input})
		if err != nil {
			return planresult.Execution{}, err
		}
		current = prepared.Session
		title := ""
		if index == 0 {
			title = "Execute plan"
		}
		if s.UserMessages != nil {
			s.UserMessages.PersistUserMessage(generationcommand.PersistUserMessage{SessionID: current.ID, Model: current.Model, Title: title, Input: input})
		}

		// ForceChatMode 跳过 Plan 提示和只读限制，避免执行阶段再次产出一份计划。
		response, err := s.Generation.Generate(ctx, generationcommand.GenerationTask{
			Session: current, LatestInput: input, Title: title, Reason: "execute plan step", Stream: command.Stream, ForceChatMode: true,
		})
		if err != nil {
			currentPlan = s.State.MarkStepFailed(currentPlan, index)
			_, _ = s.savePlanState(context.Background(), current, currentPlan, updates)
			return planresult.Execution{}, err
		}

		currentPlan = s.State.MarkStepDone(currentPlan, index)
		if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
			return planresult.Execution{}, err
		}
		combined = s.combine(combined, response)
	}

	currentPlan = s.State.MarkDone(currentPlan)
	if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
		return planresult.Execution{}, err
	}
	combined.Plan = agentplan.Clone(currentPlan)
	return combined, nil
}

func (s ExecutionService) savePlanState(ctx context.Context, current *session.Session, currentPlan *agentplan.Plan, updates planport.UpdateSink) (*agentplan.Plan, error) {
	if current == nil {
		return nil, errors.New("session is nil")
	}
	if s.PlanStates != nil {
		var err error
		currentPlan, err = s.PlanStates.Save(ctx, plancommandapp.SaveState{SessionID: current.ID, Model: current.Model, Plan: currentPlan})
		if err != nil {
			return nil, err
		}
	}
	// 每次状态变化同时更新内存、持久层、手机推送和 Hook，四处看到的是同一份 Plan 快照。
	current.CurrentPlan = agentplan.Clone(currentPlan)
	if updates != nil {
		updates.PlanUpdated(agentplan.Clone(currentPlan))
	}
	if s.Events != nil {
		s.Events.SessionChanged(ctx, current.ID, "plan")
	}
	return currentPlan, nil
}

func (s ExecutionService) combine(current planresult.Execution, next generationresult.GenerationResponse) planresult.Execution {
	if current.SessionID == "" {
		current.SessionID = next.SessionID
	}
	current.Result = s.Responses.Combine(current.Result, next.Result)
	current.Context = next.Context
	current.Compact = next.Compact
	current.Plan = next.Plan
	return current
}
