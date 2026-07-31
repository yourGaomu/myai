package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentrunapi "myai/core/application/agentrun/api"
	agentruncommand "myai/core/application/agentrun/command"
	agentrunruntime "myai/core/application/agentrun/runtime"
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
	domainagentrun "myai/core/domain/agentrun"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
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
	Runs         agentrunapi.CommandService
	OnRunError   func(error)
	State        planserviceapp.StateService
	Inputs       planserviceapp.ExecutionInputBuilder
	Responses    planserviceapp.ResponseCombiner
}

var _ planapi.Service = ExecutionService{}

func (s ExecutionService) Execute(ctx context.Context, command plancommand.Execute, updates planport.UpdateSink) (execution planresult.Execution, resultErr error) {
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
	if !agentplan.IsExecutableStatus(currentPlan.Status) {
		return planresult.Execution{}, fmt.Errorf("current plan cannot execute from status %s", currentPlan.Status)
	}

	runID := agentrunruntime.RunID(ctx)
	ownsRun := false
	currentStep := 0
	if runID == "" && s.Runs != nil {
		run, runErr := s.Runs.Start(ctx, agentruncommand.Start{
			RequestID: command.Stream.CorrelationID, SessionID: current.ID, Kind: domainagentrun.KindPlan,
			Title: "Execute plan", Reason: "execute approved plan", TotalSteps: len(currentPlan.Steps),
		})
		if runErr != nil {
			s.reportRunError(runErr)
		} else {
			runID = run.ID
			ownsRun = true
			ctx = agentrunruntime.WithRunID(ctx, runID)
			if command.Stream.OnRunStarted != nil {
				command.Stream.OnRunStarted(run)
			}
		}
	}
	if runID != "" && s.Runs != nil {
		updates = runPlanUpdateSink{
			base: updates, ctx: ctx, runID: runID, runs: s.Runs, stream: command.Stream, onError: s.reportRunError,
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
			finished, finishErr := s.Runs.Finish(context.WithoutCancel(ctx), agentruncommand.Finish{
				RunID: runID, Status: status, ErrorMessage: errorMessage,
				CurrentStep: currentStep, TotalSteps: len(currentPlan.Steps),
			})
			if finishErr != nil {
				s.reportRunError(finishErr)
				return
			}
			if command.Stream.OnRunCompleted != nil {
				command.Stream.OnRunCompleted(finished)
			}
		}()
	}

	if currentPlan.Status != agentplan.StatusApproved {
		currentPlan = s.State.Approve(currentPlan)
		if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
			return planresult.Execution{}, err
		}
	}

	// 先把整体计划置为 running 并持久化，手机端会立即收到状态更新。
	currentPlan = s.State.Start(currentPlan)
	if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
		return planresult.Execution{}, s.finishWithError(current, currentPlan, -1, err, updates)
	}

	combined := planresult.Execution{SessionID: current.ID}
	executedSteps := 0
	for index := range currentPlan.Steps {
		if currentPlan.Steps[index].Status == agentplan.StepStatusDone || currentPlan.Steps[index].Status == agentplan.StepStatusSkipped {
			currentStep = index + 1
			continue
		}
		// 每个步骤都是独立生成任务；中途取消时保留已完成步骤，并把整体计划标记为 canceled。
		if err := ctx.Err(); err != nil {
			return planresult.Execution{}, s.finishWithError(current, currentPlan, index, err, updates)
		}

		currentPlan = s.State.MarkStepRunning(currentPlan, index)
		currentStep = index + 1
		if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
			return planresult.Execution{}, s.finishWithError(current, currentPlan, index, err, updates)
		}

		// 把目标、当前步骤和完整计划转换成一条明确的用户消息，保证模型只处理当前步骤。
		input := s.Inputs.BuildStepInput(currentPlan, currentPlan.Steps[index], index, len(currentPlan.Steps))
		prepared, err := s.Messages.AppendUserMessage(ctx, messagecommand.AppendUserMessage{SessionID: current.ID, Input: input, ForceChatMode: true})
		if err != nil {
			return planresult.Execution{}, s.finishWithError(current, currentPlan, index, err, updates)
		}
		current = prepared.Session
		title := ""
		if executedSteps == 0 {
			title = "Execute plan"
		}
		if s.UserMessages != nil {
			s.UserMessages.PersistUserMessage(generationcommand.PersistUserMessage{
				SessionID: current.ID, Model: current.Model, Title: title, Input: input,
				RuntimeInstruction: prepared.RuntimeInstruction,
				SessionSnapshot:    session.Clone(current),
			})
		}

		// ForceChatMode 跳过 Plan 提示和只读限制，避免执行阶段再次产出一份计划。
		response, err := s.Generation.Generate(ctx, generationcommand.GenerationTask{
			Session: current, LatestInput: input, Title: title, Reason: "execute plan step", Stream: command.Stream, ForceChatMode: true,
		})
		if err != nil {
			return planresult.Execution{}, s.finishWithError(current, currentPlan, index, err, updates)
		}

		currentPlan = s.State.MarkStepDone(currentPlan, index)
		if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
			return planresult.Execution{}, s.finishWithError(current, currentPlan, index, err, updates)
		}
		combined = s.combine(combined, response)
		executedSteps++
	}

	currentPlan = s.State.MarkDone(currentPlan)
	if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
		return planresult.Execution{}, s.finishWithError(current, currentPlan, -1, err, updates)
	}
	combined.Plan = agentplan.Clone(currentPlan)
	return combined, nil
}

type runPlanUpdateSink struct {
	base    planport.UpdateSink
	ctx     context.Context
	runID   string
	runs    agentrunapi.CommandService
	stream  modelport.ChatStreamHandler
	onError func(error)
}

func (s runPlanUpdateSink) PlanUpdated(currentPlan *agentplan.Plan) {
	if s.base != nil {
		s.base.PlanUpdated(currentPlan)
	}
	if currentPlan == nil || s.runs == nil {
		return
	}
	currentStep, title := planProgress(currentPlan)
	event, err := s.runs.Append(context.WithoutCancel(s.ctx), agentruncommand.Append{
		RunID: s.runID, Type: domainagentrun.EventTypePlanUpdate, Title: title,
		Content: currentPlan.Goal, Status: string(currentPlan.Status),
		CurrentStep: currentStep, TotalSteps: len(currentPlan.Steps),
	})
	if err != nil {
		if s.onError != nil {
			s.onError(err)
		}
		return
	}
	if s.stream.OnRunEvent != nil {
		s.stream.OnRunEvent(event)
	}
}

func planProgress(currentPlan *agentplan.Plan) (int, string) {
	current := 0
	title := "Plan updated"
	for index, step := range currentPlan.Steps {
		if step.Status == agentplan.StepStatusRunning {
			return index + 1, step.Title
		}
		if step.Status == agentplan.StepStatusDone || step.Status == agentplan.StepStatusSkipped {
			current = index + 1
			title = step.Title
		}
	}
	return current, title
}

func (s ExecutionService) reportRunError(err error) {
	if err != nil && s.OnRunError != nil {
		s.OnRunError(err)
	}
}

func (s ExecutionService) finishWithError(current *session.Session, currentPlan *agentplan.Plan, stepIndex int, cause error, updates planport.UpdateSink) error {
	if errors.Is(cause, context.Canceled) {
		currentPlan = s.State.MarkCanceled(currentPlan)
	} else {
		currentPlan = s.State.MarkStepFailed(currentPlan, stepIndex)
	}
	if _, err := s.savePlanState(context.Background(), current, currentPlan, updates); err == nil {
		return cause
	} else {
		// A failed persistence call must not leave the live aggregate showing a
		// running Plan. Publish the terminal snapshot and report both failures.
		if current != nil {
			current.CurrentPlan = agentplan.Clone(currentPlan)
		}
		if updates != nil {
			updates.PlanUpdated(agentplan.Clone(currentPlan))
		}
		if s.Events != nil && current != nil {
			s.Events.SessionChanged(context.Background(), current.ID, "plan")
		}
		return errors.Join(cause, fmt.Errorf("save terminal plan state: %w", err))
	}
}

func (s ExecutionService) savePlanState(ctx context.Context, current *session.Session, currentPlan *agentplan.Plan, updates planport.UpdateSink) (*agentplan.Plan, error) {
	if current == nil {
		return nil, errors.New("session is nil")
	}
	if s.PlanStates != nil {
		saved, err := s.PlanStates.Save(ctx, plancommandapp.SaveState{SessionID: current.ID, Model: current.Model, Plan: currentPlan})
		if err != nil {
			return currentPlan, err
		}
		currentPlan = saved
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
