package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	agentrunmemory "myai/core/adapter/persistence/memory/agentrun"
	agentrunruntime "myai/core/application/agentrun/runtime"
	agentrunservice "myai/core/application/agentrun/service"
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	plancommand "myai/core/application/chat/plan/command"
	planresult "myai/core/application/chat/plan/result"
	messagecommand "myai/core/application/session/message/command"
	messageresult "myai/core/application/session/message/result"
	domainagentrun "myai/core/domain/agentrun"
	domaininteraction "myai/core/domain/interaction"
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	agentrunport "myai/core/port/agentrun"
	modelport "myai/core/port/model"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

func TestExecutionContinuesWhenAgentRunAppendTimesOut(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-1", SessionID: "session-1", Goal: "finish the task", Status: agentplan.StatusApproved,
			Steps: []agentplan.Step{{ID: "step-1", Order: 1, Title: "implement", Status: agentplan.StepStatusPending}},
		},
	}
	repository := blockingGetRunRepository{Repository: agentrunmemory.New()}
	runs := agentrunservice.CommandService{
		Repository: repository, IDs: &planRunIDs{}, PersistenceTimeout: 20 * time.Millisecond,
	}
	generation := &recordingGenerationService{}
	runErrors := 0
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
		Messages: fakeMessageAppender{current: current}, Generation: generation,
		Runs: runs, OnRunError: func(error) { runErrors++ },
	}

	startedAt := time.Now()
	result, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	if generation.calls != 1 {
		t.Fatalf("expected first plan step generation, got %d calls", generation.calls)
	}
	if result.Plan == nil || result.Plan.Status != agentplan.StatusDone {
		t.Fatalf("unexpected completed plan: %#v", result.Plan)
	}
	if runErrors == 0 {
		t.Fatal("expected AgentRun persistence errors to be reported")
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("AgentRun persistence delayed the plan too long: %s", elapsed)
	}
}

func TestPlanStepPromptIsMarkedInternalButRemainsInGenerationContext(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-1", SessionID: "session-1", Goal: "finish the task", Status: agentplan.StatusApproved,
			Steps: []agentplan.Step{{ID: "step-1", Order: 1, Title: "implement", Status: agentplan.StepStatusPending}},
		},
	}
	appender := &capturePlanStepAppender{current: current}
	service := ExecutionService{Messages: appender}
	task, err := service.preparePlanStep(context.Background(), current, session.Clone(current), current.CurrentPlan, 0, false)
	if err != nil {
		t.Fatalf("prepare plan step: %v", err)
	}
	if appender.command.SyntheticReason != domainmessage.SyntheticReasonPlanStep {
		t.Fatalf("plan step prompt was not marked internal: %#v", appender.command)
	}
	if len(task.Session.Messages) == 0 || !task.Session.Messages[len(task.Session.Messages)-1].IsSyntheticReason(domainmessage.SyntheticReasonPlanStep) {
		t.Fatalf("internal prompt must remain in the model context: %#v", task.Session.Messages)
	}
}

func TestExecutionPublishesPlanUpdatesWithoutRunRepository(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-1", SessionID: "session-1", Goal: "finish the task", Status: agentplan.StatusDraft,
			Steps: []agentplan.Step{{ID: "step-1", Order: 1, Title: "implement", Status: agentplan.StepStatusPending}},
		},
	}
	var updates []*agentplan.Plan
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
		Messages: fakeMessageAppender{current: current}, Generation: &recordingGenerationService{},
	}
	result, err := execution.Execute(context.Background(), plancommand.Execute{
		SessionID: current.ID,
		Stream:    modelport.ChatStreamHandler{OnPlanUpdate: func(plan *agentplan.Plan) { updates = append(updates, agentplan.Clone(plan)) }},
	}, nil)
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	assertPlanUpdateStates(t, updates, []string{
		"approved:pending", "running:pending", "running:running", "running:done", "done:done",
	})
	if result.Plan == nil || result.Plan.Status != agentplan.StatusDone {
		t.Fatalf("unexpected completed plan: %#v", result.Plan)
	}
}

func TestExecutionRunsReadyStepsSerially(t *testing.T) {
	for _, test := range []struct {
		name       string
		rawContent string
		steps      []agentplan.Step
	}{
		{
			name: "independent_and_dependent_steps",
			steps: []agentplan.Step{
				{ID: "step-1", Order: 1, Title: "first", Status: agentplan.StepStatusPending},
				{ID: "step-2", Order: 2, Title: "second", Status: agentplan.StepStatusPending},
				{ID: "step-3", Order: 3, Title: "third", Dependencies: []string{"step-1"}, Status: agentplan.StepStatusPending},
			},
		},
		{
			name:       "independent_structured_steps",
			rawContent: `{"steps":[{"id":"inspect","title":"inspect"},{"id":"verify","title":"verify"}]}`,
			steps: []agentplan.Step{
				{ID: "inspect", Order: 1, Title: "inspect", Status: agentplan.StepStatusPending},
				{ID: "verify", Order: 2, Title: "verify", Status: agentplan.StepStatusPending},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := &session.Session{
				ID: "session-1", Model: "model-1",
				CurrentPlan: &agentplan.Plan{
					ID: "plan-1", SessionID: "session-1", Goal: "ordered work", Status: agentplan.StatusApproved,
					Revision: 10, RawContent: test.rawContent, Steps: test.steps,
				},
			}
			generation := newGatedPlanGeneration(len(test.steps))
			var updates []*agentplan.Plan
			execution := ExecutionService{
				Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
				Messages: fakeMessageAppender{current: current}, Generation: generation, MaxParallelSteps: 8,
			}
			outcomes := executePlanAsync(t, execution, current.ID, modelport.ChatStreamHandler{
				OnPlanUpdate: func(plan *agentplan.Plan) { updates = append(updates, agentplan.Clone(plan)) },
			})

			statuses := make([]string, len(test.steps))
			for index := range statuses {
				statuses[index] = agentplan.StepStatusPending
			}
			wantUpdates := []string{agentplan.StatusRunning + ":" + strings.Join(statuses, ",")}
			var wantAnswers []string
			for index, step := range test.steps {
				started := awaitPlanStep(t, generation.started)
				if started.ID != step.ID {
					t.Fatalf("step %d started as %q, want %q", index+1, started.ID, step.ID)
				}
				statuses[index] = agentplan.StepStatusRunning
				wantState := agentplan.StatusRunning + ":" + strings.Join(statuses, ",")
				if got := planUpdateState(started.Task.Session.CurrentPlan); got != wantState {
					t.Fatalf("step %q generation snapshot = %q, want %q", step.ID, got, wantState)
				}
				wantUpdates = append(wantUpdates, wantState)
				assertNoPlanStepStarted(t, generation.started)
				releasePlanStep(t, generation)
				statuses[index] = agentplan.StepStatusDone
				wantUpdates = append(wantUpdates, agentplan.StatusRunning+":"+strings.Join(statuses, ","))
				wantAnswers = append(wantAnswers, step.ID+" complete")
			}
			outcome := awaitPlanOutcome(t, outcomes)
			if outcome.err != nil {
				t.Fatalf("execute plan: %v", outcome.err)
			}
			if outcome.result.Plan == nil || outcome.result.Plan.Status != agentplan.StatusDone {
				t.Fatalf("unexpected completed plan: %#v", outcome.result.Plan)
			}
			if got := outcome.result.Result.Content; got != strings.Join(wantAnswers, "\n\n") {
				t.Fatalf("combined step answers = %q, want %q", got, strings.Join(wantAnswers, "\n\n"))
			}
			wantUpdates = append(wantUpdates, agentplan.StatusDone+":"+strings.Join(statuses, ","))
			assertPlanUpdateStates(t, updates, wantUpdates)
			if updates[0].Revision != 11 || !reflect.DeepEqual(outcome.result.Plan, current.CurrentPlan) {
				t.Fatalf("live and returned plan must retain ordered revisions: live=%#v result=%#v", current.CurrentPlan, outcome.result.Plan)
			}
		})
	}
}

func TestExecutionWaitsForStepDependencies(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-1", SessionID: "session-1", Goal: "ordered work", Status: agentplan.StatusApproved,
			Steps: []agentplan.Step{
				{ID: "implement", Order: 2, Title: "implement", Dependencies: []string{"inspect"}, Status: agentplan.StepStatusPending},
				{ID: "inspect", Order: 1, Title: "inspect", Status: agentplan.StepStatusPending},
			},
		},
	}
	generation := &orderedGenerationService{steps: make(chan string, 2)}
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
		Messages: fakeMessageAppender{current: current}, Generation: generation, MaxParallelSteps: 2,
	}
	result, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	first := <-generation.steps
	second := <-generation.steps
	if first != "inspect" || second != "implement" {
		t.Fatalf("dependency order = %q, %q", first, second)
	}
	if result.Plan == nil || result.Plan.Status != agentplan.StatusDone {
		t.Fatalf("unexpected completed plan: %#v", result.Plan)
	}
}

func TestExecutionStopsAfterFirstFailedReadyStep(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-1", SessionID: "session-1", Goal: "stop failed work", Status: agentplan.StatusApproved,
			Steps: []agentplan.Step{
				{ID: "step-1", Order: 1, Title: "first", Status: agentplan.StepStatusPending},
				{ID: "step-2", Order: 2, Title: "second", Status: agentplan.StepStatusPending},
				{ID: "step-3", Order: 3, Title: "third", Dependencies: []string{"step-1"}, Status: agentplan.StepStatusPending},
			},
		},
	}
	failure := errors.New("first step failed")
	var generated []string
	messages := &appendingMessages{current: current}
	var updates []*agentplan.Plan
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current}, Messages: messages, MaxParallelSteps: 8,
		Generation: planGenerationFunc(func(ctx context.Context, _ generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
			generated = append(generated, agentrunruntime.MetadataFrom(ctx).StepID)
			return generationresult.GenerationResponse{}, failure
		}),
	}
	_, err := execution.Execute(context.Background(), plancommand.Execute{
		SessionID: current.ID,
		Stream:    modelport.ChatStreamHandler{OnPlanUpdate: func(plan *agentplan.Plan) { updates = append(updates, agentplan.Clone(plan)) }},
	}, nil)
	if !errors.Is(err, failure) {
		t.Fatalf("expected original step failure, got %v", err)
	}
	if !reflect.DeepEqual(generated, []string{"step-1"}) {
		t.Fatalf("later ready steps ran after a failure: %v", generated)
	}
	if got := planUpdateState(current.CurrentPlan); got != "failed:failed,pending,pending" {
		t.Fatalf("failed plan state = %q", got)
	}
	if current.CurrentPlan.Steps[0].LastError != failure.Error() || current.CurrentPlan.Steps[0].CompletedAt == nil {
		t.Fatalf("failed step lost failure details: %#v", current.CurrentPlan.Steps[0])
	}
	if got := len(messages.snapshot().Messages); got != 1 {
		t.Fatalf("later ready step instructions were appended after a failure: %d messages", got)
	}
	assertPlanUpdateStates(t, updates, []string{
		"running:pending,pending,pending", "running:running,pending,pending",
		"running:running,pending,pending", "failed:failed,pending,pending",
	})
}

func TestExecutionCancellationPreservesCompletedStepsAndCanResume(t *testing.T) {
	for _, cancelAt := range []int{0, 1} {
		t.Run(fmt.Sprintf("cancel_step_%d", cancelAt+1), func(t *testing.T) {
			current := &session.Session{
				ID: "session-1", Model: "model-1",
				CurrentPlan: &agentplan.Plan{
					ID: "plan-cancel", SessionID: "session-1", Goal: "cancel ordered work", Status: agentplan.StatusApproved,
					Steps: []agentplan.Step{
						{ID: "step-1", Order: 1, Title: "first", Status: agentplan.StepStatusPending},
						{ID: "step-2", Order: 2, Title: "second", Status: agentplan.StepStatusPending},
						{ID: "step-3", Order: 3, Title: "third", Dependencies: []string{"step-1"}, Status: agentplan.StepStatusPending},
					},
				},
			}
			generation := newGatedPlanGeneration(3)
			messages := &appendingMessages{current: current}
			execution := ExecutionService{
				Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
				Messages: messages, Generation: generation, MaxParallelSteps: 8,
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			outcomes := executePlanWithContextAsync(t, ctx, execution, current.ID, modelport.ChatStreamHandler{})
			for index := 0; index <= cancelAt; index++ {
				started := awaitPlanStep(t, generation.started)
				if want := fmt.Sprintf("step-%d", index+1); started.ID != want {
					t.Fatalf("step before cancellation = %q, want %q", started.ID, want)
				}
				assertNoPlanStepStarted(t, generation.started)
				if index < cancelAt {
					releasePlanStep(t, generation)
				}
			}
			cancel()
			outcome := awaitPlanOutcome(t, outcomes)
			if !errors.Is(outcome.err, context.Canceled) {
				t.Fatalf("expected context cancellation, got %v", outcome.err)
			}
			if current.CurrentPlan == nil || current.CurrentPlan.Status != agentplan.StatusCanceled {
				t.Fatalf("cancellation must leave a canceled plan: %#v", current.CurrentPlan)
			}
			for index, step := range current.CurrentPlan.Steps {
				want := agentplan.StepStatusPending
				if index < cancelAt {
					want = agentplan.StepStatusDone
				}
				if step.Status != want || step.LastError != "" {
					t.Fatalf("step %q after cancellation = %#v, want %s without failure", step.ID, step, want)
				}
				if index > cancelAt && step.StartedAt != nil {
					t.Fatalf("unstarted step acquired a start timestamp: %#v", step)
				}
			}
			if got := len(messages.snapshot().Messages); got != cancelAt+1 {
				t.Fatalf("canceled plan appended instructions for unstarted steps: got %d, want %d", got, cancelAt+1)
			}
			select {
			case started := <-generation.started:
				t.Fatalf("step %q started after cancellation", started.ID)
			default:
			}

			resumed := &orderedGenerationService{steps: make(chan string, 3)}
			execution.Generation = resumed
			result, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
			if err != nil {
				t.Fatalf("resume canceled plan: %v", err)
			}
			if result.Plan == nil || result.Plan.Status != agentplan.StatusDone {
				t.Fatalf("resumed plan did not finish: %#v", result.Plan)
			}
			var gotSteps, wantSteps []string
			for len(resumed.steps) > 0 {
				gotSteps = append(gotSteps, <-resumed.steps)
			}
			for index := cancelAt; index < 3; index++ {
				wantSteps = append(wantSteps, fmt.Sprintf("step-%d", index+1))
			}
			if !reflect.DeepEqual(gotSteps, wantSteps) {
				t.Fatalf("resumed steps = %v, want %v", gotSteps, wantSteps)
			}
		})
	}
}

func TestExecutionPreservesPermissionsAndInteractionForEveryStep(t *testing.T) {
	for _, test := range []struct {
		mode    session.PermissionMode
		ceiling []tooldef.Permission
	}{
		{mode: session.PermissionModeReadonly},
		{mode: session.PermissionModeReadWrite, ceiling: []tooldef.Permission{tooldef.PermissionRead, tooldef.PermissionWrite}},
		{mode: session.PermissionModeExecute, ceiling: []tooldef.Permission{tooldef.PermissionRead, tooldef.PermissionExecute}},
		{mode: session.PermissionModeAsk, ceiling: []tooldef.Permission{tooldef.PermissionRead}},
		{mode: session.PermissionModeFull, ceiling: []tooldef.Permission{}},
	} {
		t.Run(string(test.mode), func(t *testing.T) {
			current := &session.Session{
				ID: "session-1", Model: "model-1", Kind: session.KindSubagent,
				ParentSessionID: "parent-session", ParentTaskID: "parent-task",
				AgentDefinitionID: "restricted-agent", AgentDefinitionVer: 3,
				SystemInstruction: "keep the assigned permission boundary", AgentMode: session.AgentModePlan,
				PermissionMode: test.mode, PermissionCeiling: test.ceiling,
				AllowedTools: []string{"read_file", "shell"}, EnforceToolAllowlist: true,
				WorkspaceRoot: "test-workspace", WorkspaceSandboxID: "sandbox-1", MaxToolRounds: 7,
				CurrentPlan: &agentplan.Plan{
					ID: "plan-permissions", SessionID: "session-1", Status: agentplan.StatusApproved,
					RawContent: `{"steps":[{"id":"inspect","title":"inspect"},{"id":"verify","title":"verify"}]}`,
					Steps: []agentplan.Step{
						{ID: "inspect", Order: 1, Title: "inspect", Status: agentplan.StepStatusPending},
						{ID: "verify", Order: 2, Title: "verify", Status: agentplan.StepStatusPending},
					},
				},
			}
			wantSession := session.Clone(current)
			wantSession.CurrentPlan = nil
			var generated []string
			var permissionRequests, wantPermissions []modelport.ToolPermissionRequest
			var questions, wantQuestions []domaininteraction.Request
			stream := modelport.ChatStreamHandler{
				CorrelationID: "permission-request",
				OnToolAsk: func(request modelport.ToolPermissionRequest) bool {
					permissionRequests = append(permissionRequests, request)
					return request.Permission == tooldef.PermissionRead
				},
				OnUserQuestion: func(ctx context.Context, request domaininteraction.Request) (domaininteraction.Answer, error) {
					if ctx.Value(planStepContextKey{}) != "retained" {
						return domaininteraction.Answer{}, errors.New("step lost caller context")
					}
					questions = append(questions, request)
					return domaininteraction.Answer{Answers: map[string]string{"choice": "continue"}}, nil
				},
			}
			generation := planGenerationFunc(func(ctx context.Context, task generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
				metadata := agentrunruntime.MetadataFrom(ctx)
				generated = append(generated, metadata.StepID)
				gotSession := session.Clone(task.Session)
				gotSession.CurrentPlan = nil
				if !reflect.DeepEqual(gotSession, wantSession) {
					return generationresult.GenerationResponse{}, fmt.Errorf("step %s changed session permission or workspace boundaries: got %#v, want %#v", metadata.StepID, gotSession, wantSession)
				}
				if metadata.ParentRunID != "plan-run" || metadata.PlanID != "plan-permissions" || agentrunruntime.RunID(ctx) != "" {
					return generationresult.GenerationResponse{}, fmt.Errorf("incorrect step run metadata: %#v, run=%q", metadata, agentrunruntime.RunID(ctx))
				}
				if !task.ForceChatMode || task.Reason != "execute plan step" || task.Stream.CorrelationID != stream.CorrelationID || task.Stream.OnToolAsk == nil || task.Stream.OnUserQuestion == nil {
					return generationresult.GenerationResponse{}, errors.New("step lost execution flags or permission/interaction callbacks")
				}
				for _, permission := range []tooldef.Permission{tooldef.PermissionRead, tooldef.PermissionExecute} {
					request := modelport.ToolPermissionRequest{
						ToolCallID: metadata.StepID + "-" + string(permission), Name: "shell", Arguments: `{"command":"test-only"}`,
						Permission: permission, Mode: string(test.mode),
					}
					wantPermissions = append(wantPermissions, request)
					if allowed := task.Stream.OnToolAsk(request); allowed != (permission == tooldef.PermissionRead) {
						return generationresult.GenerationResponse{}, fmt.Errorf("step %s did not honor the %s permission decision", metadata.StepID, permission)
					}
				}
				question := domaininteraction.Request{
					ID: "question-" + metadata.StepID, SessionID: current.ID, SourceSessionID: current.ParentSessionID,
					SourceTaskID: current.ParentTaskID, RequestID: stream.CorrelationID, RunID: "step-run-" + metadata.StepID,
					ToolCallID: "question-call-" + metadata.StepID, Title: "Continue step?",
					Questions: []domaininteraction.Question{{ID: "choice", Prompt: "Continue?", Options: []string{"continue", "stop"}}},
				}
				wantQuestions = append(wantQuestions, question)
				answer, err := task.Stream.OnUserQuestion(ctx, question)
				if err != nil || answer.Cancelled || answer.Answers["choice"] != "continue" {
					return generationresult.GenerationResponse{}, fmt.Errorf("step %s lost question answer: %#v, %v", metadata.StepID, answer, err)
				}
				return generationresult.GenerationResponse{SessionID: task.Session.ID, Result: modelport.ChatResult{Content: metadata.StepID}}, nil
			})
			execution := ExecutionService{
				Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
				Messages: fakeMessageAppender{current: current}, Generation: generation, MaxParallelSteps: 8,
			}
			ctx := agentrunruntime.WithRunID(context.WithValue(context.Background(), planStepContextKey{}, "retained"), "plan-run")
			result, err := execution.Execute(ctx, plancommand.Execute{SessionID: current.ID, Stream: stream}, nil)
			if err != nil {
				t.Fatalf("execute plan: %v", err)
			}
			if !reflect.DeepEqual(generated, []string{"inspect", "verify"}) || result.Plan == nil || result.Plan.Status != agentplan.StatusDone {
				t.Fatalf("permission checks must run for every step: generated=%v plan=%#v", generated, result.Plan)
			}
			if len(permissionRequests) != 4 || !reflect.DeepEqual(permissionRequests, wantPermissions) {
				t.Fatalf("permission requests changed: got %#v, want %#v", permissionRequests, wantPermissions)
			}
			if len(questions) != 2 || !reflect.DeepEqual(questions, wantQuestions) {
				t.Fatalf("user questions changed: got %#v, want %#v", questions, wantQuestions)
			}
		})
	}
}

func assertPlanUpdateStates(t *testing.T, updates []*agentplan.Plan, want []string) {
	t.Helper()
	got := make([]string, len(updates))
	for index, update := range updates {
		got[index] = planUpdateState(update)
		if index > 0 && update.Revision != updates[index-1].Revision+1 {
			t.Errorf("plan revision %d = %d, previous = %d", index, update.Revision, updates[index-1].Revision)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plan updates = %v, want %v", got, want)
	}
}

func planUpdateState(plan *agentplan.Plan) string {
	if plan == nil {
		return "<nil>"
	}
	statuses := make([]string, len(plan.Steps))
	for index, step := range plan.Steps {
		statuses[index] = step.Status
	}
	return plan.Status + ":" + strings.Join(statuses, ",")
}

type planExecutionOutcome struct {
	result planresult.Execution
	err    error
}

func executePlanAsync(t *testing.T, execution ExecutionService, sessionID string, stream modelport.ChatStreamHandler) <-chan planExecutionOutcome {
	t.Helper()
	return executePlanWithContextAsync(t, context.Background(), execution, sessionID, stream)
}

func executePlanWithContextAsync(t *testing.T, ctx context.Context, execution ExecutionService, sessionID string, stream modelport.ChatStreamHandler) <-chan planExecutionOutcome {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	outcomes := make(chan planExecutionOutcome, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		result, err := execution.Execute(ctx, plancommand.Execute{SessionID: sessionID, Stream: stream}, nil)
		outcomes <- planExecutionOutcome{result: result, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("plan execution did not stop during cleanup")
		}
	})
	return outcomes
}

func awaitPlanOutcome(t *testing.T, outcomes <-chan planExecutionOutcome) planExecutionOutcome {
	t.Helper()
	select {
	case outcome := <-outcomes:
		return outcome
	case <-time.After(2 * time.Second):
		t.Fatal("plan execution did not finish")
		return planExecutionOutcome{}
	}
}

type startedPlanStep struct {
	ID   string
	Task generationcommand.GenerationTask
}

type gatedPlanGenerationService struct {
	started  chan startedPlanStep
	release  chan struct{}
	complete func(string, generationcommand.GenerationTask)
}

func newGatedPlanGeneration(steps int) *gatedPlanGenerationService {
	return &gatedPlanGenerationService{started: make(chan startedPlanStep, steps), release: make(chan struct{})}
}

func (service *gatedPlanGenerationService) Generate(ctx context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	metadata := agentrunruntime.MetadataFrom(ctx)
	service.started <- startedPlanStep{ID: metadata.StepID, Task: command}
	select {
	case <-service.release:
	case <-ctx.Done():
		return generationresult.GenerationResponse{}, ctx.Err()
	}
	if service.complete != nil {
		service.complete(metadata.StepID, command)
	}
	return generationresult.GenerationResponse{SessionID: command.Session.ID, Result: modelport.ChatResult{Content: metadata.StepID + " complete"}}, nil
}

func awaitPlanStep(t *testing.T, started <-chan startedPlanStep) startedPlanStep {
	t.Helper()
	select {
	case step := <-started:
		return step
	case <-time.After(2 * time.Second):
		t.Fatal("expected next plan step to start")
		return startedPlanStep{}
	}
}

func assertNoPlanStepStarted(t *testing.T, started <-chan startedPlanStep) {
	t.Helper()
	select {
	case step := <-started:
		t.Fatalf("step %q started before the preceding step finished", step.ID)
	case <-time.After(20 * time.Millisecond):
	}
}

func releasePlanStep(t *testing.T, generation *gatedPlanGenerationService) {
	t.Helper()
	select {
	case generation.release <- struct{}{}:
	case <-time.After(2 * time.Second):
		t.Fatal("plan step did not wait for release")
	}
}

type planGenerationFunc func(context.Context, generationcommand.GenerationTask) (generationresult.GenerationResponse, error)

func (generate planGenerationFunc) Generate(ctx context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	return generate(ctx, command)
}

type planStepContextKey struct{}

type fakeModelProvider struct{}

func (fakeModelProvider) GetModel(string) modelport.ChatModelPort { return nil }

type fakeSessionLoader struct{ current *session.Session }

func (loader fakeSessionLoader) Load(context.Context, string) (*session.Session, error) {
	return loader.current, nil
}

type fakeMessageAppender struct{ current *session.Session }

func (appender fakeMessageAppender) AppendUserMessage(_ context.Context, command messagecommand.AppendUserMessage) (messageresult.Command, error) {
	return messageresult.Command{Session: appender.current, Input: command.Input, Appended: true}, nil
}

type capturePlanStepAppender struct {
	current *session.Session
	command messagecommand.AppendUserMessage
}

func (appender *capturePlanStepAppender) AppendUserMessage(_ context.Context, command messagecommand.AppendUserMessage) (messageresult.Command, error) {
	appender.command = command
	message := domainmessage.SyntheticUserText(command.SyntheticReason, command.Input)
	current := session.Clone(appender.current)
	current.AppendMessage(message)
	return messageresult.Command{
		Session: current, Input: command.Input, Appended: true,
		AppendedMessages: []domainmessage.Message{message},
	}, nil
}

type recordingGenerationService struct{ calls int }

func (service *recordingGenerationService) Generate(_ context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	service.calls++
	return generationresult.GenerationResponse{
		SessionID: command.Session.ID,
		Result:    modelport.ChatResult{Content: "step complete"},
	}, nil
}

type orderedGenerationService struct{ steps chan string }

func (service *orderedGenerationService) Generate(ctx context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	metadata := agentrunruntime.MetadataFrom(ctx)
	service.steps <- metadata.StepID
	return generationresult.GenerationResponse{SessionID: command.Session.ID, Result: modelport.ChatResult{Content: command.LatestInput}}, nil
}

type blockingGetRunRepository struct{ agentrunport.Repository }

func (blockingGetRunRepository) GetRun(ctx context.Context, _ string) (domainagentrun.Run, error) {
	<-ctx.Done()
	return domainagentrun.Run{}, ctx.Err()
}

type planRunIDs struct{ next int }

func (ids *planRunIDs) NewID() string {
	ids.next++
	return fmt.Sprintf("run-id-%d", ids.next)
}
