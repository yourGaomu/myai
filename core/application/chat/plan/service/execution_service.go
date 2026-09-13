package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

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
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type ExecutionService struct {
	// ExecutionService 执行已经保存在 Session.CurrentPlan 中的计划；草稿计划会在执行前自动批准。
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
	Recovery     planport.RecoveryPlanner
	MaxReplans   int
	// MaxParallelSteps limits the number of dependency-ready steps executed in
	// one batch. Zero preserves the legacy sequential behavior.
	MaxParallelSteps int
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
			RequestID: command.Stream.CorrelationID, SessionID: current.ID, ParentRunID: command.ParentRunID, PlanID: currentPlan.ID, Kind: domainagentrun.KindPlan,
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
	if command.Stream.OnPlanUpdate != nil {
		updates = streamPlanUpdateSink{base: updates, stream: command.Stream}
	}
	if runID != "" && s.Runs != nil {
		updates = runPlanUpdateSink{
			base: updates, ctx: ctx, runID: runID, runs: s.Runs, stream: command.Stream, onError: s.reportRunError,
		}
	}
	ctx = agentrunruntime.WithMetadata(ctx, agentrunruntime.Metadata{RunID: runID, ParentRunID: command.ParentRunID, PlanID: currentPlan.ID})
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

	combined := planresult.Execution{SessionID: current.ID, RunID: runID}
	executedSteps := 0
	replans := 0
	// A plan is executed in dependency-ready batches. A zero MaxParallelSteps
	// intentionally retains the old one-step-at-a-time behavior for callers
	// that have not opted into parallel execution.
	for {
		if err := ctx.Err(); err != nil {
			return planresult.Execution{}, s.finishWithError(current, currentPlan, currentStep-1, err, updates)
		}
		ready := readyStepIndexes(currentPlan)
		if len(ready) == 0 {
			if allStepsTerminal(currentPlan) {
				break
			}
			err := errors.New("plan has an unsatisfied or cyclic step dependency")
			return planresult.Execution{}, s.finishWithError(current, currentPlan, firstUnfinishedStep(currentPlan), err, updates)
		}
		if limit := s.maxParallelSteps(currentPlan); len(ready) > limit {
			ready = ready[:limit]
		}
		for _, index := range ready {
			currentPlan = s.State.MarkStepRunning(currentPlan, index)
			currentStep = index + 1
		}
		if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
			return planresult.Execution{}, s.finishWithError(current, currentPlan, ready[0], err, updates)
		}

		batchResults := s.executeReadyBatch(ctx, current, currentPlan, ready, runID, command.Stream, executedSteps == 0)
		sort.Slice(batchResults, func(left, right int) bool { return batchResults[left].Index < batchResults[right].Index })
		var firstFailure *stepExecutionResult
		failureCount := 0
		for index := range batchResults {
			item := &batchResults[index]
			if item.Err != nil {
				failureCount++
				if firstFailure == nil {
					firstFailure = item
				}
				continue
			}
			if item.Output != nil {
				item.Output.Flush(command.Stream)
			}
			currentPlan = s.State.MarkStepDone(currentPlan, item.Index)
			combined = s.combine(combined, item.Response)
			executedSteps++
		}
		if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
			return planresult.Execution{}, s.finishWithError(current, currentPlan, -1, err, updates)
		}
		if firstFailure == nil {
			continue
		}
		index := firstFailure.Index
		if failureCount > 1 {
			if errors.Is(firstFailure.Err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				// Cancellation is a resumable pause, even when several parallel
				// branches observe it at once. Do not convert those branches into
				// terminal failures.
				return planresult.Execution{}, s.finishWithError(current, currentPlan, firstFailure.Index, firstFailure.Err, updates)
			}
			// A parallel batch can fail in more than one independent branch. Do
			// not retry or recover only the first branch while leaving another
			// branch in running state; mark every failed branch terminal so a
			// resume has an honest, deterministic starting point.
			for _, item := range batchResults {
				if item.Err != nil {
					currentPlan = s.State.MarkStepFailed(currentPlan, item.Index, item.Err.Error())
				}
			}
			if _, saveErr := s.savePlanState(ctx, current, currentPlan, updates); saveErr != nil {
				return planresult.Execution{}, s.finishWithError(current, currentPlan, index, errors.Join(firstFailure.Err, saveErr), updates)
			}
			return planresult.Execution{}, firstFailure.Err
		}
		if maxRetries := currentPlan.Steps[index].MaxRetries; maxRetries > 0 && currentPlan.Steps[index].RetryCount < maxRetries && ctx.Err() == nil {
			currentPlan = s.State.MarkStepRetry(currentPlan, index, firstFailure.Err.Error())
			if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
				return planresult.Execution{}, s.finishWithError(current, currentPlan, index, err, updates)
			}
			continue
		}
		if s.Recovery != nil && replans < s.maxReplans() && ctx.Err() == nil {
			replacement, recoveryErr := s.Recovery.Recover(ctx, plancommand.RecoveryRequest{
				Plan: currentPlan, Step: currentPlan.Steps[index], Error: firstFailure.Err.Error(), Attempt: replans + 1,
			})
			if recoveryErr == nil && replacement != nil && len(replacement.Steps) > 0 {
				currentPlan = mergeRecoveryPlan(currentPlan, replacement, index, firstFailure.Err.Error())
				replans++
				if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
					return planresult.Execution{}, s.finishWithError(current, currentPlan, index, err, updates)
				}
				continue
			}
		}
		return planresult.Execution{}, s.finishWithError(current, currentPlan, index, firstFailure.Err, updates)
	}

	currentPlan = s.State.MarkDone(currentPlan)
	if currentPlan, err = s.savePlanState(ctx, current, currentPlan, updates); err != nil {
		return planresult.Execution{}, s.finishWithError(current, currentPlan, -1, err, updates)
	}
	combined.Plan = agentplan.Clone(currentPlan)
	return combined, nil
}

func (s ExecutionService) maxReplans() int {
	if s.MaxReplans > 0 {
		return s.MaxReplans
	}
	return 1
}

func (s ExecutionService) maxParallelSteps(currentPlan *agentplan.Plan) int {
	// Plans produced before dependency metadata was introduced are implicitly
	// ordered by their list position. Keep those plans serial even when the
	// application enables parallel scheduling for newer structured plans.
	if !planHasDependencies(currentPlan) {
		return 1
	}
	if s.MaxParallelSteps > 0 {
		return s.MaxParallelSteps
	}
	return 1
}

func planHasDependencies(currentPlan *agentplan.Plan) bool {
	if currentPlan == nil {
		return false
	}
	for _, step := range currentPlan.Steps {
		if len(step.Dependencies) > 0 {
			return true
		}
	}
	// A structured plan can legitimately contain only independent steps. The
	// dependency slices are then empty, so use the preserved raw payload to
	// distinguish it from legacy Markdown plans, which remain sequential for
	// backwards compatibility.
	return len(agentplan.ExtractSteps(currentPlan.RawContent)) == len(currentPlan.Steps) &&
		strings.Contains(currentPlan.RawContent, "\"steps\"")
}

type stepExecutionResult struct {
	Index    int
	Response generationresult.GenerationResponse
	Err      error
	Output   *bufferedStepOutput
}

func (s ExecutionService) executeReadyBatch(ctx context.Context, current *session.Session, currentPlan *agentplan.Plan, indexes []int, runID string, stream modelport.ChatStreamHandler, titleFirst bool) []stepExecutionResult {
	results := make([]stepExecutionResult, len(indexes))
	var appendMu sync.Mutex
	serializedStream := synchronizedStream{base: stream}
	parallel := len(indexes) > 1
	var waitGroup sync.WaitGroup
	for resultIndex, stepIndex := range indexes {
		resultIndex, stepIndex := resultIndex, stepIndex
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			stepStream := serializedStream.Handler()
			var output *bufferedStepOutput
			if parallel {
				output = &bufferedStepOutput{}
				stepStream = output.Handler(stepStream)
			}
			results[resultIndex] = s.executePlanStep(ctx, current, currentPlan, stepIndex, runID, stepStream, &appendMu, titleFirst && resultIndex == 0)
			results[resultIndex].Output = output
		}()
	}
	waitGroup.Wait()
	return results
}

type bufferedStepOutput struct {
	mu        sync.Mutex
	reasoning []string
	answer    []string
}

func (b *bufferedStepOutput) Handler(base modelport.ChatStreamHandler) modelport.ChatStreamHandler {
	if b == nil {
		return base
	}
	handler := base
	handler.OnReasoning = func(text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		b.mu.Lock()
		b.reasoning = append(b.reasoning, text)
		b.mu.Unlock()
	}
	handler.OnAnswer = func(text string) {
		if text == "" {
			return
		}
		b.mu.Lock()
		b.answer = append(b.answer, text)
		b.mu.Unlock()
	}
	return handler
}

func (b *bufferedStepOutput) Flush(stream modelport.ChatStreamHandler) {
	if b == nil {
		return
	}
	b.mu.Lock()
	reasoning := append([]string(nil), b.reasoning...)
	answer := append([]string(nil), b.answer...)
	b.mu.Unlock()
	if stream.OnReasoning != nil {
		for _, text := range reasoning {
			stream.OnReasoning(text)
		}
	}
	if stream.OnAnswer != nil {
		for _, text := range answer {
			stream.OnAnswer(text)
		}
	}
}

// synchronizedStream keeps transport callbacks and permission prompts
// serialized when multiple dependency-ready steps finish concurrently. The
// underlying model/tool execution remains parallel, but WebSocket writers and
// UI state consumers receive a coherent callback sequence.
type synchronizedStream struct {
	mu   sync.Mutex
	base modelport.ChatStreamHandler
}

func (s *synchronizedStream) Handler() modelport.ChatStreamHandler {
	if s == nil {
		return modelport.ChatStreamHandler{}
	}
	handler := s.base
	if s.base.OnReasoning != nil {
		handler.OnReasoning = func(text string) { s.mu.Lock(); defer s.mu.Unlock(); s.base.OnReasoning(text) }
	}
	if s.base.OnAnswer != nil {
		handler.OnAnswer = func(text string) { s.mu.Lock(); defer s.mu.Unlock(); s.base.OnAnswer(text) }
	}
	if s.base.OnToolCall != nil {
		handler.OnToolCall = func(name string, arguments string) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.base.OnToolCall(name, arguments)
		}
	}
	if s.base.OnToolResult != nil {
		handler.OnToolResult = func(event modelport.ToolResultEvent) { s.mu.Lock(); defer s.mu.Unlock(); s.base.OnToolResult(event) }
	}
	if s.base.OnToolAsk != nil {
		handler.OnToolAsk = func(request modelport.ToolPermissionRequest) bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.base.OnToolAsk(request)
		}
	}
	if s.base.OnRunStarted != nil {
		handler.OnRunStarted = func(run domainagentrun.Run) { s.mu.Lock(); defer s.mu.Unlock(); s.base.OnRunStarted(run) }
	}
	if s.base.OnRunEvent != nil {
		handler.OnRunEvent = func(event domainagentrun.Event) { s.mu.Lock(); defer s.mu.Unlock(); s.base.OnRunEvent(event) }
	}
	if s.base.OnRunCompleted != nil {
		handler.OnRunCompleted = func(run domainagentrun.Run) { s.mu.Lock(); defer s.mu.Unlock(); s.base.OnRunCompleted(run) }
	}
	if s.base.OnPlanUpdate != nil {
		handler.OnPlanUpdate = func(plan *agentplan.Plan) { s.mu.Lock(); defer s.mu.Unlock(); s.base.OnPlanUpdate(plan) }
	}
	return handler
}

func (s ExecutionService) executePlanStep(ctx context.Context, current *session.Session, currentPlan *agentplan.Plan, index int, runID string, stream modelport.ChatStreamHandler, appendMu *sync.Mutex, titleFirst bool) stepExecutionResult {
	result := stepExecutionResult{Index: index}
	step := currentPlan.Steps[index]
	input := s.Inputs.BuildStepInput(currentPlan, step, index, len(currentPlan.Steps))
	// Message append and its persistence snapshot are serialized because most
	// session adapters expose a single mutable aggregate. The model/tool turn
	// itself runs outside this critical section, allowing independent steps to
	// overlap.
	appendMu.Lock()
	prepared, err := s.Messages.AppendUserMessage(ctx, messagecommand.AppendUserMessage{SessionID: current.ID, Input: input, ForceChatMode: true})
	if err == nil && prepared.Session == nil {
		err = errors.New("message appender returned nil session")
	}
	if err == nil && s.UserMessages != nil {
		title := ""
		if titleFirst {
			title = "Execute plan"
		}
		s.UserMessages.PersistUserMessage(generationcommand.PersistUserMessage{
			SessionID: prepared.Session.ID, Model: prepared.Session.Model, Title: title, Input: input,
			RuntimeInstruction: prepared.RuntimeInstruction, AppendedMessages: domainmessage.CloneAll(prepared.AppendedMessages), SessionSnapshot: session.Clone(prepared.Session),
		})
	}
	appendMu.Unlock()
	if err != nil {
		result.Err = err
		return result
	}
	stepSession := session.Clone(prepared.Session)
	stepContext := agentrunruntime.WithMetadata(ctx, agentrunruntime.Metadata{
		ParentRunID: runID, PlanID: currentPlan.ID, StepID: step.ID,
	})
	stepContext = agentrunruntime.WithRunID(stepContext, "")
	title := ""
	if titleFirst {
		title = "Execute plan"
	}
	result.Response, result.Err = s.Generation.Generate(stepContext, generationcommand.GenerationTask{
		Session: stepSession, LatestInput: input, Title: title, Reason: "execute plan step", Stream: stream, ForceChatMode: true,
	})
	return result
}

func readyStepIndexes(currentPlan *agentplan.Plan) []int {
	if currentPlan == nil {
		return nil
	}
	ready := make([]int, 0, len(currentPlan.Steps))
	for index, step := range currentPlan.Steps {
		if step.Status != agentplan.StepStatusPending {
			continue
		}
		if dependenciesSatisfied(currentPlan, step) {
			ready = append(ready, index)
		}
	}
	return ready
}

func dependenciesSatisfied(currentPlan *agentplan.Plan, step agentplan.Step) bool {
	for _, dependency := range step.Dependencies {
		dependency = strings.TrimSpace(dependency)
		found := false
		for _, candidate := range currentPlan.Steps {
			if candidate.ID != dependency && fmt.Sprintf("%d", candidate.Order) != dependency {
				continue
			}
			found = true
			if candidate.Status != agentplan.StepStatusDone && candidate.Status != agentplan.StepStatusSkipped {
				return false
			}
			break
		}
		if !found {
			return false
		}
	}
	return true
}

func allStepsTerminal(currentPlan *agentplan.Plan) bool {
	if currentPlan == nil {
		return true
	}
	for _, step := range currentPlan.Steps {
		if step.Status != agentplan.StepStatusDone && step.Status != agentplan.StepStatusSkipped {
			return false
		}
	}
	return true
}

func firstUnfinishedStep(currentPlan *agentplan.Plan) int {
	if currentPlan == nil {
		return -1
	}
	for index, step := range currentPlan.Steps {
		if step.Status != agentplan.StepStatusDone && step.Status != agentplan.StepStatusSkipped {
			return index
		}
	}
	return -1
}

func mergeRecoveryPlan(current *agentplan.Plan, replacement *agentplan.Plan, failedIndex int, failure string) *agentplan.Plan {
	merged := agentplan.Clone(current)
	if merged == nil {
		return agentplan.Clone(replacement)
	}
	if failedIndex >= 0 && failedIndex < len(merged.Steps) {
		merged.Steps[failedIndex].Status = agentplan.StepStatusSkipped
		merged.Steps[failedIndex].LastError = strings.TrimSpace(failure)
	}
	failedTitle := ""
	if failedIndex >= 0 && failedIndex < len(merged.Steps) {
		failedTitle = strings.ToLower(strings.TrimSpace(merged.Steps[failedIndex].Title))
	}
	seen := make(map[string]struct{}, len(merged.Steps))
	for _, step := range merged.Steps {
		seen[strings.ToLower(strings.TrimSpace(step.Title))] = struct{}{}
	}
	for _, step := range replacement.Steps {
		titleKey := strings.ToLower(strings.TrimSpace(step.Title))
		if titleKey == "" || titleKey == failedTitle {
			continue
		}
		if _, exists := seen[titleKey]; exists {
			continue
		}
		step.ID = strings.TrimSpace(step.ID)
		if step.ID == "" {
			step.ID = uuid.NewString()
		}
		step.Order = len(merged.Steps) + 1
		step.Status = agentplan.StepStatusPending
		step.RetryCount = 0
		step.LastError = ""
		merged.Steps = append(merged.Steps, step)
		seen[titleKey] = struct{}{}
		if len(merged.Steps) >= 12 {
			break
		}
	}
	merged.Status = agentplan.StatusRunning
	if strings.TrimSpace(replacement.RawContent) != "" {
		merged.RawContent = strings.TrimSpace(merged.RawContent + "\n\nRecovery plan:\n" + replacement.RawContent)
	}
	return merged
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

type streamPlanUpdateSink struct {
	base   planport.UpdateSink
	stream modelport.ChatStreamHandler
}

func (s streamPlanUpdateSink) PlanUpdated(currentPlan *agentplan.Plan) {
	if s.base != nil {
		s.base.PlanUpdated(currentPlan)
	}
	if s.stream.OnPlanUpdate != nil {
		s.stream.OnPlanUpdate(agentplan.Clone(currentPlan))
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
		currentPlan = s.State.MarkStepFailed(currentPlan, stepIndex, cause.Error())
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
	} else {
		currentPlan = agentplan.Clone(currentPlan)
		if currentPlan != nil {
			currentPlan.Revision++
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
