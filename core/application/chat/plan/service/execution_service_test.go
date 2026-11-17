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
	planstatecommand "myai/core/application/plan/command"
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

func TestExecutionRejectsInvalidGraphBeforeAnySideEffects(t *testing.T) {
	for _, test := range []struct {
		name  string
		steps []agentplan.Step
		want  string
	}{
		{name: "unknown_dependency_after_ready_step", want: "unknown", steps: []agentplan.Step{
			{ID: "ready", Order: 1, Title: "ready", Status: agentplan.StepStatusPending},
			{ID: "blocked", Order: 2, Title: "blocked", Dependencies: []string{"missing"}, Status: agentplan.StepStatusPending},
		}},
		{name: "cycle_after_ready_step", want: "cyclic", steps: []agentplan.Step{
			{ID: "ready", Order: 1, Title: "ready", Status: agentplan.StepStatusPending},
			{ID: "a", Order: 2, Title: "a", Dependencies: []string{"b"}, Status: agentplan.StepStatusPending},
			{ID: "b", Order: 3, Title: "b", Dependencies: []string{"a"}, Status: agentplan.StepStatusPending},
		}},
		{name: "self_dependency", want: "self", steps: []agentplan.Step{
			{ID: "a", Order: 1, Title: "a", Dependencies: []string{"a"}, Status: agentplan.StepStatusPending},
		}},
		{name: "duplicate_id", want: "duplicate step ID", steps: []agentplan.Step{
			{ID: "a", Order: 1, Title: "first", Status: agentplan.StepStatusPending},
			{ID: "a", Order: 2, Title: "second", Status: agentplan.StepStatusPending},
		}},
		{name: "duplicate_order", want: "duplicate order", steps: []agentplan.Step{
			{ID: "a", Order: 5, Title: "first", Status: agentplan.StepStatusPending},
			{ID: "b", Order: 5, Title: "second", Status: agentplan.StepStatusPending},
			{ID: "c", Order: 6, Title: "third", Dependencies: []string{"5"}, Status: agentplan.StepStatusPending},
		}},
		{name: "ambiguous_id_order", want: "ambiguous", steps: []agentplan.Step{
			{ID: "2", Order: 1, Title: "first", Status: agentplan.StepStatusPending},
			{ID: "b", Order: 2, Title: "second", Status: agentplan.StepStatusPending},
			{ID: "c", Order: 3, Title: "third", Dependencies: []string{"2"}, Status: agentplan.StepStatusPending},
		}},
		{name: "empty_id", want: "ID", steps: []agentplan.Step{{Order: 1, Title: "first"}}},
		{name: "invalid_order", want: "order", steps: []agentplan.Step{{ID: "a", Title: "first"}}},
		{name: "empty_dependency", want: "empty", steps: []agentplan.Step{{ID: "a", Order: 1, Title: "first", Dependencies: []string{" "}}}},
		{name: "unknown_status", want: "status", steps: []agentplan.Step{{ID: "a", Order: 1, Title: "first", Status: "unexpected"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := &session.Session{ID: "session-1", Model: "model-1", CurrentPlan: &agentplan.Plan{
				ID: "invalid", SessionID: "session-1", Status: agentplan.StatusDraft, Revision: 7, Steps: test.steps,
			}}
			before := session.Clone(current)
			models := &countingPlanModels{}
			messages := &appendingMessages{current: current}
			generation := &recordingGenerationService{}
			states := &countingPlanStates{}
			userMessages := &acceptingPlanUserMessages{}
			events := &countingPlanEvents{}
			runIDs := &planRunIDs{}
			updates, runs, tools, recoveries := 0, 0, 0, 0
			execution := ExecutionService{
				Models: models, Sessions: fakeSessionLoader{current: current}, Messages: messages,
				Generation: generation, PlanStates: states, UserMessages: userMessages, Events: events,
				Runs: agentrunservice.CommandService{Repository: agentrunmemory.New(), IDs: runIDs},
				Recovery: planRecoveryFunc(func(context.Context, plancommand.RecoveryRequest) (*agentplan.Plan, error) {
					recoveries++
					return nil, nil
				}),
			}
			_, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID, Stream: modelport.ChatStreamHandler{
				OnPlanUpdate: func(*agentplan.Plan) { updates++ },
				OnRunStarted: func(domainagentrun.Run) { runs++ },
				OnToolCall:   func(string, string) { tools++ },
			}}, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("invalid graph error = %v, want %q", err, test.want)
			}
			if models.calls != 0 || generation.calls != 0 || tools != 0 || len(messages.commands) != 0 || states.calls != 0 || userMessages.accepted != 0 || userMessages.legacy != 0 || events.count != 0 || updates != 0 || runs != 0 || runIDs.next != 0 || recoveries != 0 {
				t.Fatalf("invalid graph produced side effects: model=%d generation=%d tools=%d messages=%d states=%d accepted=%d legacy=%d events=%d updates=%d runs=%d runIDs=%d recovery=%d", models.calls, generation.calls, tools, len(messages.commands), states.calls, userMessages.accepted, userMessages.legacy, events.count, updates, runs, runIDs.next, recoveries)
			}
			if !reflect.DeepEqual(session.Clone(current), before) {
				t.Fatal("invalid graph mutated the loaded session")
			}
		})
	}
}

func TestExecutionIgnoresInjectedCompletionAndSupportsLegacyDependencies(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    []string
	}{
		{name: "injected_completion", content: `{"status":"done","steps":[{"id":"first","title":"First","status":"done"},{"id":"second","title":"Second","status":"skipped","depends_on":["first"]}]}`, want: []string{"first", "second"}},
		{name: "legacy_orders", content: `{"steps":[{"id":"second","order":2,"title":"Second","dependencies":["1"]},{"id":"first","order":1,"title":"First"}]}`, want: []string{"first", "second"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := &session.Session{ID: "session-1", Model: "model-1", CurrentPlan: agentplan.NewDraft("session-1", "work", test.content, time.Now())}
			var generated []string
			execution := ExecutionService{
				Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current}, Messages: fakeMessageAppender{current: current},
				Generation: planGenerationFunc(func(ctx context.Context, task generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
					generated = append(generated, agentrunruntime.MetadataFrom(ctx).StepID)
					return generationresult.GenerationResponse{SessionID: task.Session.ID}, nil
				}),
			}
			result, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
			if err != nil || result.Plan == nil || result.Plan.Status != agentplan.StatusDone || !reflect.DeepEqual(generated, test.want) {
				t.Fatalf("expected every real step in dependency order: result=%#v steps=%v err=%v", result.Plan, generated, err)
			}
		})
	}
}

func TestExecutionRejectsLoadedRunningPlanOrStep(t *testing.T) {
	for _, planRunning := range []bool{false, true} {
		current := &session.Session{ID: "session-1", CurrentPlan: &agentplan.Plan{
			ID: "plan", Status: agentplan.StatusApproved,
			Steps: []agentplan.Step{{ID: "work", Order: 1, Title: "Work", Status: agentplan.StepStatusRunning}},
		}}
		if planRunning {
			current.CurrentPlan.Status = agentplan.StatusRunning
			current.CurrentPlan.Steps[0].Status = agentplan.StepStatusPending
		}
		before := session.Clone(current)
		generation := &recordingGenerationService{}
		messages := &appendingMessages{current: current}
		states := &countingPlanStates{}
		_, err := (ExecutionService{
			Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current}, Messages: messages, Generation: generation, PlanStates: states,
		}).Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
		if err == nil || !strings.Contains(err.Error(), "interrupted running") || generation.calls != 0 || states.calls != 0 || len(messages.commands) != 0 || !reflect.DeepEqual(session.Clone(current), before) {
			t.Fatalf("interrupted execution was restarted: generation=%d messages=%d states=%d err=%v", generation.calls, len(messages.commands), states.calls, err)
		}
	}
}

type countingPlanModels struct{ calls int }

func (models *countingPlanModels) GetModel(string) modelport.ChatModelPort {
	models.calls++
	return nil
}

type countingPlanStates struct {
	calls int
	err   error
	plans []*agentplan.Plan
}

func (states *countingPlanStates) Save(_ context.Context, command planstatecommand.SaveState) (*agentplan.Plan, error) {
	states.calls++
	states.plans = append(states.plans, agentplan.Clone(command.Plan))
	if states.err != nil {
		return nil, states.err
	}
	p := agentplan.Clone(command.Plan)
	p.Revision++
	return p, nil
}

type acceptingPlanUserMessages struct {
	accepted int
	legacy   int
	err      error
	commands []generationcommand.PersistUserMessage
}

func (messages *acceptingPlanUserMessages) PersistUserMessage(generationcommand.PersistUserMessage) {
	messages.legacy++
}

func (messages *acceptingPlanUserMessages) AcceptUserMessage(_ context.Context, command generationcommand.PersistUserMessage) error {
	messages.accepted++
	messages.commands = append(messages.commands, command)
	return messages.err
}

type countingPlanEvents struct{ count int }

func (events *countingPlanEvents) SessionChanged(context.Context, string, string) { events.count++ }

type planRecoveryFunc func(context.Context, plancommand.RecoveryRequest) (*agentplan.Plan, error)

func (recover planRecoveryFunc) Recover(ctx context.Context, command plancommand.RecoveryRequest) (*agentplan.Plan, error) {
	return recover(ctx, command)
}

func TestExecutionRecoveryPreservesDependenciesAndWaitsForEveryTerminal(t *testing.T) {
	for _, diamond := range []bool{false, true} {
		t.Run(fmt.Sprintf("diamond_%t", diamond), func(t *testing.T) {
			current := &session.Session{ID: "session-1", Model: "model-1", CurrentPlan: &agentplan.Plan{
				ID: "plan", SessionID: "session-1", Status: agentplan.StatusApproved,
				Steps: []agentplan.Step{
					{ID: "upstream", Order: 1, Title: "Upstream", Status: agentplan.StepStatusPending},
					{ID: "failed", Order: 2, Title: "Original work", Dependencies: []string{"1"}, Status: agentplan.StepStatusPending},
					{ID: "downstream", Order: 3, Title: "Downstream", Dependencies: []string{"2"}, Status: agentplan.StepStatusPending},
				},
			}}
			replacement := &agentplan.Plan{RawContent: "recovery steps", Steps: []agentplan.Step{
				{ID: "left", Order: 1, Title: "Alternate left", Status: agentplan.StepStatusDone},
				{ID: "right", Order: 2, Title: "Alternate right", Status: agentplan.StepStatusSkipped},
			}}
			want := []string{"upstream", "failed", "left", "right", "downstream"}
			terminals := []string{"left", "right"}
			if diamond {
				replacement.Steps = append(replacement.Steps, agentplan.Step{ID: "join", Order: 3, Title: "Alternate join", Dependencies: []string{"left", "right"}})
				want = []string{"upstream", "failed", "left", "right", "join", "downstream"}
				terminals = []string{"join"}
			}
			replacementBefore := agentplan.Clone(replacement)
			failure := errors.New("original failed")
			var generated []string
			recoveryCalls := 0
			execution := ExecutionService{
				Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current}, Messages: &appendingMessages{current: current}, MaxParallelSteps: 8,
				Generation: planGenerationFunc(func(ctx context.Context, task generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
					id := agentrunruntime.MetadataFrom(ctx).StepID
					generated = append(generated, id)
					if id == "failed" {
						return generationresult.GenerationResponse{}, failure
					}
					if id != "upstream" {
						if err := agentplan.Validate(task.Session.CurrentPlan); err != nil {
							return generationresult.GenerationResponse{}, fmt.Errorf("invalid executing recovery graph: %w", err)
						}
						for _, step := range task.Session.CurrentPlan.Steps {
							if step.ID == "failed" {
								return generationresult.GenerationResponse{}, errors.New("failed step was not replaced")
							}
						}
					}
					if id == "downstream" {
						for _, terminal := range terminals {
							index, err := agentplan.DependencyIndex(task.Session.CurrentPlan, terminal)
							if err != nil || task.Session.CurrentPlan.Steps[index].Status != agentplan.StepStatusDone {
								return generationresult.GenerationResponse{}, errors.New("downstream started before replacement terminal completed")
							}
						}
					}
					return generationresult.GenerationResponse{SessionID: task.Session.ID, Result: modelport.ChatResult{Content: id}}, nil
				}),
				Recovery: planRecoveryFunc(func(_ context.Context, request plancommand.RecoveryRequest) (*agentplan.Plan, error) {
					recoveryCalls++
					if request.Step.ID != "failed" || request.Attempt != 1 || request.Error != failure.Error() || request.Plan.Steps[0].Status != agentplan.StepStatusDone {
						t.Errorf("incorrect recovery request: %#v", request)
					}
					return replacement, nil
				}),
			}
			result, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
			if err != nil || result.Plan == nil || result.Plan.Status != agentplan.StatusDone || recoveryCalls != 1 || !reflect.DeepEqual(generated, want) {
				t.Fatalf("recovery execution order: steps=%v want=%v calls=%d result=%#v err=%v", generated, want, recoveryCalls, result.Plan, err)
			}
			for _, id := range []string{"left", "right"} {
				index, _ := agentplan.DependencyIndex(result.Plan, id)
				if !reflect.DeepEqual(result.Plan.Steps[index].Dependencies, []string{"upstream"}) {
					t.Fatalf("replacement %s lost upstream dependencies: %#v", id, result.Plan.Steps[index])
				}
			}
			downstream, _ := agentplan.DependencyIndex(result.Plan, "downstream")
			if !reflect.DeepEqual(result.Plan.Steps[downstream].Dependencies, terminals) {
				t.Fatalf("downstream must wait for replacement terminals: %#v", result.Plan.Steps[downstream])
			}
			if !reflect.DeepEqual(replacement, replacementBefore) {
				t.Fatal("merge changed the recovery planner's candidate")
			}
		})
	}
}

func TestExecutionRejectsInvalidRecoveryWithoutRunningLaterWork(t *testing.T) {
	for _, test := range []struct {
		name  string
		steps []agentplan.Step
		want  string
	}{
		{name: "all_replacements_deduplicated", want: "duplicate", steps: []agentplan.Step{{ID: "again", Title: " Original work "}}},
		{name: "partly_duplicate_is_not_partial_success", want: "duplicate", steps: []agentplan.Step{{ID: "new", Title: "Alternate work"}, {ID: "again", Title: "Downstream"}}},
		{name: "unknown_dependency", want: "unknown", steps: []agentplan.Step{{ID: "new", Title: "Alternate work", Dependencies: []string{"missing"}}}},
		{name: "removed_failed_dependency", want: "replaces", steps: []agentplan.Step{{ID: "new", Title: "Alternate work", Dependencies: []string{"failed"}}}},
		{name: "cycle", want: "cyclic", steps: []agentplan.Step{{ID: "a", Title: "Alternate A", Dependencies: []string{"b"}}, {ID: "b", Title: "Alternate B", Dependencies: []string{"a"}}}},
		{name: "self_dependency", want: "self", steps: []agentplan.Step{{ID: "a", Title: "Alternate A", Dependencies: []string{"a"}}}},
		{name: "duplicate_id", want: "duplicate step ID", steps: []agentplan.Step{{ID: "a", Title: "Alternate A"}, {ID: "a", Title: "Alternate B"}}},
		{name: "duplicate_order", want: "duplicate order", steps: []agentplan.Step{{ID: "a", Order: 2, Title: "Alternate A"}, {ID: "b", Order: 2, Title: "Alternate B"}}},
		{name: "cycle_through_downstream", want: "cyclic", steps: []agentplan.Step{{ID: "new", Title: "Alternate work", Dependencies: []string{"downstream"}}}},
		{name: "empty", want: "no replacement"},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := &session.Session{ID: "session-1", Model: "model-1", CurrentPlan: &agentplan.Plan{
				ID: "plan", SessionID: "session-1", Status: agentplan.StatusApproved,
				Steps: []agentplan.Step{
					{ID: "failed", Order: 1, Title: "Original work", Status: agentplan.StepStatusPending},
					{ID: "downstream", Order: 2, Title: "Downstream", Dependencies: []string{"failed"}, Status: agentplan.StepStatusPending},
				},
			}}
			failure := errors.New("original step failed")
			var generated []string
			messages := &appendingMessages{current: current}
			recoveries := 0
			execution := ExecutionService{
				Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current}, Messages: messages,
				Generation: planGenerationFunc(func(ctx context.Context, _ generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
					generated = append(generated, agentrunruntime.MetadataFrom(ctx).StepID)
					return generationresult.GenerationResponse{}, failure
				}),
				Recovery: planRecoveryFunc(func(context.Context, plancommand.RecoveryRequest) (*agentplan.Plan, error) {
					recoveries++
					return &agentplan.Plan{Steps: test.steps}, nil
				}),
			}
			_, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), test.want) || recoveries != 1 || !reflect.DeepEqual(generated, []string{"failed"}) || len(messages.commands) != 1 {
				t.Fatalf("invalid recovery ran later work or lost failure: steps=%v recovery=%d messages=%d err=%v", generated, recoveries, len(messages.commands), err)
			}
			if got := planUpdateState(current.CurrentPlan); got != "failed:failed,pending" || current.CurrentPlan.Steps[0].ID != "failed" {
				t.Fatalf("invalid recovery must leave original failure visible: %s %#v", got, current.CurrentPlan)
			}
		})
	}
}

func TestExecutionDoesNotCompleteWhenEverySingleStepReplacementIsDuplicate(t *testing.T) {
	current := &session.Session{ID: "session-1", CurrentPlan: &agentplan.Plan{
		ID: "plan", SessionID: "session-1", Status: agentplan.StatusApproved,
		Steps: []agentplan.Step{{ID: "original", Order: 1, Title: "Work", Status: agentplan.StepStatusPending}},
	}}
	failure := errors.New("work failed")
	calls, recoveryCalls := 0, 0
	var updates []*agentplan.Plan
	_, err := (ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current}, Messages: fakeMessageAppender{current: current},
		Generation: planGenerationFunc(func(context.Context, generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
			calls++
			return generationresult.GenerationResponse{}, failure
		}),
		Recovery: planRecoveryFunc(func(context.Context, plancommand.RecoveryRequest) (*agentplan.Plan, error) {
			recoveryCalls++
			return &agentplan.Plan{Steps: []agentplan.Step{{ID: "retry", Title: " work "}, {ID: "retry-2", Title: "WORK"}}}, nil
		}),
	}).Execute(context.Background(), plancommand.Execute{SessionID: current.ID, Stream: modelport.ChatStreamHandler{OnPlanUpdate: func(p *agentplan.Plan) { updates = append(updates, agentplan.Clone(p)) }}}, nil)
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "duplicate") || calls != 1 || recoveryCalls != 1 || planUpdateState(current.CurrentPlan) != "failed:failed" || current.CurrentPlan.Steps[0].ID != "original" {
		t.Fatalf("duplicate-only recovery hid the original failed work: calls=%d recovery=%d plan=%#v err=%v", calls, recoveryCalls, current.CurrentPlan, err)
	}
	for _, update := range updates {
		if update.Status == agentplan.StatusDone || update.Steps[0].Status == agentplan.StepStatusSkipped {
			t.Fatalf("duplicate-only recovery published false completion: %#v", update)
		}
	}
}

func TestExecutionRecoveryFailureKeepsDownstreamBlockedAndCanResume(t *testing.T) {
	current := &session.Session{ID: "session-1", CurrentPlan: &agentplan.Plan{
		ID: "plan", SessionID: "session-1", Status: agentplan.StatusApproved,
		Steps: []agentplan.Step{
			{ID: "failed", Order: 1, Title: "Original work", Status: agentplan.StepStatusPending},
			{ID: "downstream", Order: 2, Title: "Downstream", Dependencies: []string{"failed"}, Status: agentplan.StepStatusPending},
		},
	}}
	failure := errors.New("step failed")
	var generated []string
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current}, Messages: fakeMessageAppender{current: current},
		Generation: planGenerationFunc(func(ctx context.Context, _ generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
			generated = append(generated, agentrunruntime.MetadataFrom(ctx).StepID)
			return generationresult.GenerationResponse{}, failure
		}),
		Recovery: planRecoveryFunc(func(context.Context, plancommand.RecoveryRequest) (*agentplan.Plan, error) {
			return &agentplan.Plan{Steps: []agentplan.Step{{ID: "alternate", Title: "Alternate work"}}}, nil
		}),
	}
	_, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
	if !errors.Is(err, failure) || !reflect.DeepEqual(generated, []string{"failed", "alternate"}) || planUpdateState(current.CurrentPlan) != "failed:failed,pending" || current.CurrentPlan.Steps[0].ID != "alternate" {
		t.Fatalf("failed replacement allowed downstream work: generated=%v plan=%#v err=%v", generated, current.CurrentPlan, err)
	}
	generated = nil
	execution.Recovery = nil
	execution.Generation = planGenerationFunc(func(ctx context.Context, task generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
		generated = append(generated, agentrunruntime.MetadataFrom(ctx).StepID)
		return generationresult.GenerationResponse{SessionID: task.Session.ID}, nil
	})
	result, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
	if err != nil || result.Plan.Status != agentplan.StatusDone || !reflect.DeepEqual(generated, []string{"alternate", "downstream"}) {
		t.Fatalf("resume failed replacement: generated=%v result=%#v err=%v", generated, result.Plan, err)
	}
}

func TestMergeRecoveryPlanRemapsIDsAndPreservesInputs(t *testing.T) {
	now := time.Now()
	current := &agentplan.Plan{Status: agentplan.StatusRunning, Steps: []agentplan.Step{
		{ID: "upstream", Order: 1, Title: "Upstream", Status: agentplan.StepStatusDone},
		{ID: "failed", Order: 2, Title: "Original work", Dependencies: []string{"upstream"}, Status: agentplan.StepStatusRunning},
		{ID: "downstream", Order: 3, Title: "Downstream", Dependencies: []string{"2"}, Status: agentplan.StepStatusPending},
	}}
	replacement := &agentplan.Plan{Steps: []agentplan.Step{
		{ID: "upstream", Order: 1, Title: "Alternate work", Status: agentplan.StepStatusDone, RetryCount: 9, LastError: "old", AgentTaskID: "old-task", StartedAt: &now, CompletedAt: &now},
		{ID: "finish", Order: 2, Title: "Alternate finish", Dependencies: []string{"upstream"}},
	}}
	before, replacementBefore := agentplan.Clone(current), agentplan.Clone(replacement)
	merged, err := mergeRecoveryPlan(current, replacement, 1, "failed")
	if err != nil || merged == nil {
		t.Fatalf("merge collision: %v", err)
	}
	if err := agentplan.Validate(merged); err != nil {
		t.Fatalf("merged graph is invalid: %v", err)
	}
	first := merged.Steps[1]
	if first.ID == "upstream" || first.ID == "failed" || first.Status != agentplan.StepStatusPending || first.RetryCount != 0 || first.LastError != "" || first.AgentTaskID != "" || first.StartedAt != nil || first.CompletedAt != nil || !reflect.DeepEqual(first.Dependencies, []string{"upstream"}) {
		t.Fatalf("replacement retained stale identity/state or lost upstream: %#v", first)
	}
	if !reflect.DeepEqual(merged.Steps[2].Dependencies, []string{"upstream", first.ID}) || !reflect.DeepEqual(merged.Steps[3].Dependencies, []string{"finish"}) {
		t.Fatalf("replacement ID remapping lost edges: %#v", merged.Steps)
	}
	if !reflect.DeepEqual(current, before) || !reflect.DeepEqual(replacement, replacementBefore) {
		t.Fatal("merge must not mutate either input")
	}
}

func TestMergeRecoveryPlanChecksLimitBeforeReplacing(t *testing.T) {
	current := &agentplan.Plan{Status: agentplan.StatusRunning}
	for index := 0; index < 12; index++ {
		current.Steps = append(current.Steps, agentplan.Step{ID: fmt.Sprintf("s%d", index), Order: index + 1, Title: fmt.Sprintf("Step %d", index), Status: agentplan.StepStatusPending})
	}
	before := agentplan.Clone(current)
	replacement := &agentplan.Plan{Steps: []agentplan.Step{{ID: "r1", Title: "Alternate first"}, {ID: "r2", Title: "Alternate second"}}}
	if merged, err := mergeRecoveryPlan(current, replacement, 0, "failed"); err == nil || merged != nil || !reflect.DeepEqual(current, before) {
		t.Fatalf("oversized replacement was partially merged: %#v err=%v", merged, err)
	}
	replacement.Steps = replacement.Steps[:1]
	merged, err := mergeRecoveryPlan(current, replacement, 0, "failed")
	if err != nil || len(merged.Steps) != 12 || merged.Steps[0].ID != "r1" {
		t.Fatalf("one-for-one replacement at the limit must work: %#v err=%v", merged, err)
	}
}

func TestExecutionStopsBeforeGenerationWhenInputPersistenceFails(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure error
		status  string
	}{
		{name: "failed", failure: errors.New("user queue rejected input"), status: agentplan.StatusFailed},
		{name: "canceled", failure: context.Canceled, status: agentplan.StatusCanceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := &session.Session{ID: "session-1", Model: "model-1", CurrentPlan: &agentplan.Plan{
				ID: "plan", SessionID: "session-1", Status: agentplan.StatusApproved,
				Steps: []agentplan.Step{
					{ID: "first", Order: 1, Title: "First", Status: agentplan.StepStatusPending, MaxRetries: 3},
					{ID: "second", Order: 2, Title: "Second", Status: agentplan.StepStatusPending},
				},
			}}
			generation := &recordingGenerationService{}
			messages := &appendingMessages{current: current, runtimeInstruction: true}
			userMessages := &acceptingPlanUserMessages{err: test.failure}
			states := &countingPlanStates{}
			recoveryCalls, tools := 0, 0
			execution := ExecutionService{
				Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current}, Messages: messages,
				Generation: generation, UserMessages: userMessages, PlanStates: states,
				Recovery: planRecoveryFunc(func(context.Context, plancommand.RecoveryRequest) (*agentplan.Plan, error) {
					recoveryCalls++
					return nil, nil
				}),
			}
			_, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID, Stream: modelport.ChatStreamHandler{OnToolCall: func(string, string) { tools++ }}}, nil)
			if !errors.Is(err, test.failure) || !errors.Is(err, errPlanInputPersistence) || generation.calls != 0 || userMessages.accepted != 1 || userMessages.legacy != 0 || recoveryCalls != 0 || tools != 0 || len(messages.commands) != 1 {
				t.Fatalf("input rejection did not stop the execution: generation=%d accepted=%d legacy=%d recovery=%d tools=%d messages=%d err=%v", generation.calls, userMessages.accepted, userMessages.legacy, recoveryCalls, tools, len(messages.commands), err)
			}
			if current.CurrentPlan.Status != test.status || states.plans[len(states.plans)-1].Status != test.status || current.CurrentPlan.Steps[0].RetryCount != 0 || current.CurrentPlan.Steps[1].StartedAt != nil {
				t.Fatalf("input rejection left incorrect terminal state: %#v", current.CurrentPlan)
			}
			command := userMessages.commands[0]
			if command.SessionSnapshot == current || command.SessionID != current.ID || command.Model != current.Model || command.SyntheticReason != domainmessage.SyntheticReasonPlanStep || len(command.AppendedMessages) != 2 || command.RuntimeInstruction == "" || command.Input != command.AppendedMessages[1].Text() {
				t.Fatalf("input acceptance did not receive the prepared turn: %#v", command)
			}
		})
	}
}

func TestExecutionReportsInputAndTerminalPersistenceFailures(t *testing.T) {
	inputErr, stateErr := errors.New("input rejected"), errors.New("plan state unavailable")
	current := &session.Session{ID: "session-1", CurrentPlan: &agentplan.Plan{ID: "plan", Status: agentplan.StatusApproved, Steps: []agentplan.Step{{ID: "first", Order: 1, Title: "First", Status: agentplan.StepStatusPending}}}}
	userMessages := &acceptingPlanUserMessages{err: inputErr}
	states := &countingPlanStates{}
	generation := &recordingGenerationService{}
	updates := 0
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current}, Messages: fakeMessageAppender{current: current},
		Generation: generation, UserMessages: userMessages, PlanStates: states,
	}
	_, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID, Stream: modelport.ChatStreamHandler{OnPlanUpdate: func(plan *agentplan.Plan) {
		updates++
		if len(plan.Steps) == 1 && plan.Steps[0].Status == agentplan.StepStatusRunning {
			states.err = stateErr
		}
	}}}, nil)
	if !errors.Is(err, inputErr) || !errors.Is(err, stateErr) || generation.calls != 0 || userMessages.accepted != 1 || current.CurrentPlan.Status != agentplan.StatusFailed || updates < 3 {
		t.Fatalf("input/terminal persistence failure was lost: generation=%d accepted=%d plan=%#v updates=%d err=%v", generation.calls, userMessages.accepted, current.CurrentPlan, updates, err)
	}
}

func TestRecoveryPlannerAcceptsPreparedInputBeforeGeneration(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprintf("reject_%t", reject), func(t *testing.T) {
			current := &session.Session{ID: "session-1", Model: "model-1", AgentMode: session.AgentModeChat}
			messages := &appendingMessages{current: current, runtimeInstruction: true}
			inputErr := errors.New("recovery queue rejected input")
			userMessages := &acceptingPlanUserMessages{}
			if reject {
				userMessages.err = inputErr
			}
			calls := 0
			planner := GenerationRecoveryPlanner{
				Messages: messages, UserMessages: userMessages,
				Generation: planGenerationFunc(func(_ context.Context, task generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
					calls++
					if userMessages.accepted != 1 || !task.Internal || !task.CapturePlan || task.ForceChatMode || task.Session.AgentMode != session.AgentModePlan {
						return generationresult.GenerationResponse{}, errors.New("recovery lost acceptance order or planning boundary")
					}
					return generationresult.GenerationResponse{Plan: agentplan.NewDraft(current.ID, "recover", "## Plan\n1. Alternate work", time.Now())}, nil
				}),
			}
			result, err := planner.Recover(context.Background(), plancommand.RecoveryRequest{
				Plan: &agentplan.Plan{ID: "plan", SessionID: current.ID}, Step: agentplan.Step{ID: "failed", Title: "Original"}, Attempt: 1,
			})
			if reject {
				if !errors.Is(err, inputErr) || result != nil || calls != 0 {
					t.Fatalf("rejected recovery input generated work: result=%#v calls=%d err=%v", result, calls, err)
				}
			} else if err != nil || result == nil || len(result.Steps) != 1 || calls != 1 {
				t.Fatalf("accepted recovery input did not generate: result=%#v calls=%d err=%v", result, calls, err)
			}
			if userMessages.accepted != 1 || userMessages.legacy != 0 || len(messages.commands) != 1 || current.AgentMode != session.AgentModeChat {
				t.Fatalf("recovery acceptance duplicated input or changed live mode: accepted=%d legacy=%d messages=%d mode=%s", userMessages.accepted, userMessages.legacy, len(messages.commands), current.AgentMode)
			}
			command := userMessages.commands[0]
			if command.SyntheticReason != domainmessage.SyntheticReasonAutonomousPlanning || len(command.AppendedMessages) != 2 || command.RuntimeInstruction == "" || command.SessionSnapshot == current {
				t.Fatalf("recovery acceptance lost prepared context: %#v", command)
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
