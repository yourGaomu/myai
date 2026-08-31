package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	agentplan "myai/core/plan"
	agentrunport "myai/core/port/agentrun"
	modelport "myai/core/port/model"
	"myai/core/session"
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
	if len(updates) != 5 {
		t.Fatalf("expected 5 plan updates, got %d", len(updates))
	}
	if updates[0].Status != agentplan.StatusApproved || updates[1].Status != agentplan.StatusRunning || updates[len(updates)-1].Status != agentplan.StatusDone {
		t.Fatalf("unexpected plan update states: %#v", updates)
	}
	if result.Plan == nil || result.Plan.Status != agentplan.StatusDone {
		t.Fatalf("unexpected completed plan: %#v", result.Plan)
	}
}

func TestExecutionRunsIndependentReadyStepsInParallel(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-1", SessionID: "session-1", Goal: "parallel work", Status: agentplan.StatusApproved,
			Steps: []agentplan.Step{
				{ID: "step-1", Order: 1, Title: "first", Status: agentplan.StepStatusPending},
				{ID: "step-2", Order: 2, Title: "second", Status: agentplan.StepStatusPending},
				{ID: "step-3", Order: 3, Title: "third", Dependencies: []string{"step-1"}, Status: agentplan.StepStatusPending},
			},
		},
	}
	generation := &parallelGenerationService{started: make(chan string, 3), release: make(chan struct{})}
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
		Messages: fakeMessageAppender{current: current}, Generation: generation, MaxParallelSteps: 2,
	}
	resultCh := make(chan struct {
		result planresult.Execution
		err    error
	}, 1)
	go func() {
		result, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
		resultCh <- struct {
			result planresult.Execution
			err    error
		}{result: result, err: err}
	}()

	for index := 0; index < 2; index++ {
		select {
		case <-generation.started:
		case <-time.After(time.Second):
			t.Fatal("expected two dependency-ready steps to start concurrently")
		}
	}
	select {
	case step := <-generation.started:
		t.Fatalf("third step started before the first batch completed: %s", step)
	case <-time.After(40 * time.Millisecond):
	}
	close(generation.release)
	select {
	case outcome := <-resultCh:
		if outcome.err != nil {
			t.Fatalf("execute plan: %v", outcome.err)
		}
		if outcome.result.Plan == nil || outcome.result.Plan.Status != agentplan.StatusDone {
			t.Fatalf("unexpected completed plan: %#v", outcome.result.Plan)
		}
	case <-time.After(time.Second):
		t.Fatal("parallel plan execution did not finish")
	}
}

func TestExecutionRunsIndependentStructuredStepsInParallel(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-structured", SessionID: "session-1", Goal: "independent work", Status: agentplan.StatusApproved,
			RawContent: `{"steps":[{"id":"inspect","title":"inspect"},{"id":"verify","title":"verify"}]}`,
			Steps: []agentplan.Step{
				{ID: "inspect", Order: 1, Title: "inspect", Status: agentplan.StepStatusPending},
				{ID: "verify", Order: 2, Title: "verify", Status: agentplan.StepStatusPending},
			},
		},
	}
	generation := &parallelGenerationService{started: make(chan string, 2), release: make(chan struct{})}
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
		Messages: fakeMessageAppender{current: current}, Generation: generation, MaxParallelSteps: 2,
	}
	resultCh := make(chan error, 1)
	go func() {
		_, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
		resultCh <- err
	}()
	for index := 0; index < 2; index++ {
		select {
		case <-generation.started:
		case <-time.After(time.Second):
			t.Fatal("expected independent structured steps to start concurrently")
		}
	}
	close(generation.release)
	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("execute plan: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("structured parallel plan execution did not finish")
	}
}

func TestExecutionWaitsForStepDependencies(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-1", SessionID: "session-1", Goal: "ordered work", Status: agentplan.StatusApproved,
			Steps: []agentplan.Step{
				{ID: "inspect", Order: 1, Title: "inspect", Status: agentplan.StepStatusPending},
				{ID: "implement", Order: 2, Title: "implement", Dependencies: []string{"inspect"}, Status: agentplan.StepStatusPending},
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

func TestExecutionMarksAllFailedBranchesWhenParallelBatchFails(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-1", SessionID: "session-1", Goal: "parallel failure", Status: agentplan.StatusApproved,
			Steps: []agentplan.Step{
				{ID: "step-1", Order: 1, Title: "first", Status: agentplan.StepStatusPending},
				{ID: "step-2", Order: 2, Title: "second", Status: agentplan.StepStatusPending},
				{ID: "step-3", Order: 3, Title: "third", Dependencies: []string{"step-1"}, Status: agentplan.StepStatusPending},
			},
		},
	}
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
		Messages: fakeMessageAppender{current: current}, Generation: &parallelFailingGenerationService{}, MaxParallelSteps: 2,
	}
	_, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
	if err == nil {
		t.Fatal("expected parallel batch failure")
	}
	if current.CurrentPlan == nil || current.CurrentPlan.Status != agentplan.StatusFailed {
		t.Fatalf("unexpected failed plan: %#v", current.CurrentPlan)
	}
	if current.CurrentPlan.Steps[0].Status != agentplan.StepStatusFailed || current.CurrentPlan.Steps[1].Status != agentplan.StepStatusFailed {
		t.Fatalf("all failed branches must be terminal: %#v", current.CurrentPlan.Steps)
	}
}

func TestExecutionMarksParallelCancellationAsCanceled(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-cancel", SessionID: "session-1", Goal: "cancel parallel work", Status: agentplan.StatusApproved,
			Steps: []agentplan.Step{
				{ID: "step-1", Order: 1, Title: "first", Status: agentplan.StepStatusPending},
				{ID: "step-2", Order: 2, Title: "second", Status: agentplan.StepStatusPending},
				{ID: "step-3", Order: 3, Title: "third", Dependencies: []string{"step-1"}, Status: agentplan.StepStatusPending},
			},
		},
	}
	generation := &parallelGenerationService{started: make(chan string, 3), release: make(chan struct{})}
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
		Messages: fakeMessageAppender{current: current}, Generation: generation, MaxParallelSteps: 2,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := make(chan error, 1)
	go func() {
		_, err := execution.Execute(ctx, plancommand.Execute{SessionID: current.ID}, nil)
		resultCh <- err
	}()
	for index := 0; index < 2; index++ {
		select {
		case <-generation.started:
		case <-time.After(time.Second):
			t.Fatal("expected two parallel steps before cancellation")
		}
	}
	cancel()
	select {
	case err := <-resultCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled parallel plan execution did not finish")
	}
	if current.CurrentPlan == nil || current.CurrentPlan.Status != agentplan.StatusCanceled {
		t.Fatalf("parallel cancellation must leave a canceled plan: %#v", current.CurrentPlan)
	}
	for _, step := range current.CurrentPlan.Steps {
		if step.Status == agentplan.StepStatusRunning || step.Status == agentplan.StepStatusFailed {
			t.Fatalf("canceled plan contains non-resumable step state: %#v", current.CurrentPlan.Steps)
		}
	}
}

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

type recordingGenerationService struct{ calls int }

func (service *recordingGenerationService) Generate(_ context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	service.calls++
	return generationresult.GenerationResponse{
		SessionID: command.Session.ID,
		Result:    modelport.ChatResult{Content: "step complete"},
	}, nil
}

type parallelGenerationService struct {
	mu      sync.Mutex
	started chan string
	release chan struct{}
}

func (service *parallelGenerationService) Generate(ctx context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	metadata := agentrunruntime.MetadataFrom(ctx)
	service.mu.Lock()
	service.started <- metadata.StepID
	service.mu.Unlock()
	select {
	case <-service.release:
	case <-ctx.Done():
		return generationresult.GenerationResponse{}, ctx.Err()
	}
	return generationresult.GenerationResponse{SessionID: command.Session.ID, Result: modelport.ChatResult{Content: metadata.StepID}}, nil
}

type orderedGenerationService struct{ steps chan string }

func (service *orderedGenerationService) Generate(ctx context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	metadata := agentrunruntime.MetadataFrom(ctx)
	service.steps <- metadata.StepID
	return generationresult.GenerationResponse{SessionID: command.Session.ID, Result: modelport.ChatResult{Content: command.LatestInput}}, nil
}

type parallelFailingGenerationService struct{}

func (*parallelFailingGenerationService) Generate(_ context.Context, _ generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	return generationresult.GenerationResponse{}, errors.New("parallel step failed")
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
