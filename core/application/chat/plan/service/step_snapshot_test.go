package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	agentrunruntime "myai/core/application/agentrun/runtime"
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	plancommand "myai/core/application/chat/plan/command"
	messagecommand "myai/core/application/session/message/command"
	messageresult "myai/core/application/session/message/result"
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

type appendingMessages struct {
	mu                 sync.Mutex
	current            *session.Session
	commands           []messagecommand.AppendUserMessage
	runtimeInstruction bool
}

func (a *appendingMessages) AppendUserMessage(ctx context.Context, c messagecommand.AppendUserMessage) (messageresult.Command, error) {
	if err := ctx.Err(); err != nil {
		return messageresult.Command{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.SessionID != a.current.ID {
		return messageresult.Command{}, fmt.Errorf("unexpected session %q", c.SessionID)
	}
	a.commands = append(a.commands, c)
	instruction := ""
	if a.runtimeInstruction {
		instruction = fmt.Sprintf("runtime rules for append %d", len(a.commands))
	}
	start := len(a.current.Messages)
	a.current.AddUserTurnWithReason("", instruction, c.Input, c.SyntheticReason)
	return messageresult.Command{
		Session: session.Clone(a.current), Input: c.Input, RuntimeInstruction: instruction,
		AppendedMessages: domainmessage.CloneAll(a.current.Messages[start:]), Appended: true,
	}, nil
}

func (a *appendingMessages) appendCommitted(messages ...domainmessage.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.current.AppendMessages(messages...)
}

func (a *appendingMessages) snapshot() *session.Session {
	a.mu.Lock()
	defer a.mu.Unlock()
	return session.Clone(a.current)
}

func TestReadyBatchSeriallyCarriesCompleteToolRoundTrips(t *testing.T) {
	current := &session.Session{ID: "session", Model: "model"}
	current.AppendMessages(
		domainmessage.Text(domainmessage.RoleSystem, "system rules"),
		domainmessage.Text(domainmessage.RoleUser, "shared history"),
	)
	frozen := session.Clone(current)
	messages := &appendingMessages{current: current, runtimeInstruction: true}
	plan := &agentplan.Plan{
		ID: "plan", SessionID: current.ID,
		Steps: []agentplan.Step{
			{ID: "a", Order: 1, Title: "first", Status: agentplan.StepStatusPending},
			{ID: "b", Order: 2, Title: "second", Status: agentplan.StepStatusPending},
			{ID: "c", Order: 3, Title: "third", Status: agentplan.StepStatusPending},
		},
	}
	generation := newGatedPlanGeneration(3)
	generation.complete = func(stepID string, _ generationcommand.GenerationTask) {
		messages.appendCommitted(planStepRoundTrip(stepID)...)
	}
	service := ExecutionService{Messages: messages, Generation: generation, MaxParallelSteps: 8}
	ctx, cancel := context.WithCancel(context.Background())
	outcomes := make(chan []stepExecutionResult, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		outcomes <- service.executeReadyBatch(ctx, frozen, plan, []int{0, 1, 2}, "plan-run", modelport.ChatStreamHandler{}, true)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("ready batch did not stop during cleanup")
		}
	})

	var snapshots []*session.Session
	for index, step := range plan.Steps {
		started := awaitPlanStep(t, generation.started)
		if started.ID != step.ID {
			t.Fatalf("batch step %d = %q, want %q", index+1, started.ID, step.ID)
		}
		want := messages.snapshot()
		if !reflect.DeepEqual(started.Task.Session, want) {
			t.Fatalf("step %q did not receive the latest committed transcript: got %#v, want %#v", step.ID, started.Task.Session.Messages, want.Messages)
		}
		if got, expected := len(started.Task.Session.Messages), 2+index*5+2; got != expected {
			t.Fatalf("step %q context has %d messages, want %d without duplicated instructions", step.ID, got, expected)
		}
		assertCompletePlanTranscript(t, started.Task.Session.Messages, index+1, index)
		if last := started.Task.Session.Messages[len(started.Task.Session.Messages)-1]; last.Text() != started.Task.LatestInput || !last.IsSyntheticReason(domainmessage.SyntheticReasonPlanStep) {
			t.Fatalf("step %q lost its current internal instruction: %#v", step.ID, last)
		}
		wantTitle := ""
		if index == 0 {
			wantTitle = "Execute plan"
		}
		if started.Task.Title != wantTitle || !started.Task.ForceChatMode {
			t.Fatalf("step %q execution metadata changed: %#v", step.ID, started.Task)
		}
		snapshots = append(snapshots, session.Clone(started.Task.Session))
		assertNoPlanStepStarted(t, generation.started)
		releasePlanStep(t, generation)
	}
	var results []stepExecutionResult
	select {
	case results = <-outcomes:
	case <-time.After(2 * time.Second):
		t.Fatal("serial ready batch did not finish")
	}
	if len(results) != 3 {
		t.Fatalf("batch produced %d results, want 3", len(results))
	}
	for index, result := range results {
		if result.Err != nil || result.Index != index || result.Response.Result.Content != plan.Steps[index].ID+" complete" {
			t.Fatalf("batch result %d = %#v", index, result)
		}
	}
	latest := messages.snapshot()
	if len(latest.Messages) != 17 {
		t.Fatalf("committed transcript has %d messages, want 17", len(latest.Messages))
	}
	assertCompletePlanTranscript(t, latest.Messages, 3, 3)
	for index, snapshot := range snapshots {
		if !reflect.DeepEqual(snapshot.Messages, latest.Messages[:len(snapshot.Messages)]) {
			t.Fatalf("step %d changed earlier message identities or tool data", index+1)
		}
	}
	if !reflect.DeepEqual(frozen.Messages, latest.Messages[:2]) || len(messages.commands) != 3 {
		t.Fatalf("batch rewrote its old snapshot or appended duplicate turns: frozen=%#v commands=%#v", frozen.Messages, messages.commands)
	}
}

func TestExecutionNextStepSeesCommittedAssistantAndToolResults(t *testing.T) {
	current := &session.Session{
		ID: "session", Model: "model",
		CurrentPlan: &agentplan.Plan{
			ID: "plan", SessionID: "session", Goal: "use earlier findings", Status: agentplan.StatusApproved,
			RawContent: `{"steps":[{"id":"inspect","title":"inspect"},{"id":"verify","title":"verify"}]}`,
			Steps: []agentplan.Step{
				{ID: "inspect", Order: 1, Title: "inspect", Status: agentplan.StepStatusPending},
				{ID: "verify", Order: 2, Title: "verify", Status: agentplan.StepStatusPending},
			},
		},
	}
	current.AppendMessages(domainmessage.Text(domainmessage.RoleSystem, "system rules"), domainmessage.Text(domainmessage.RoleUser, "shared history"))
	messages := &appendingMessages{current: current, runtimeInstruction: true}
	var snapshots []*session.Session
	generation := planGenerationFunc(func(ctx context.Context, task generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
		index := len(snapshots)
		stepID := agentrunruntime.MetadataFrom(ctx).StepID
		if index >= 2 || stepID != current.CurrentPlan.Steps[index].ID {
			return generationresult.GenerationResponse{}, fmt.Errorf("unexpected generated step %q at %d", stepID, index)
		}
		if !reflect.DeepEqual(task.Session, messages.snapshot()) {
			return generationresult.GenerationResponse{}, fmt.Errorf("step %q missed committed assistant/tool context", stepID)
		}
		wantState := "running:running,pending"
		if index == 1 {
			wantState = "running:done,running"
		}
		if got := planUpdateState(task.Session.CurrentPlan); got != wantState {
			return generationresult.GenerationResponse{}, fmt.Errorf("step %q plan snapshot = %s, want %s", stepID, got, wantState)
		}
		assertCompletePlanTranscript(t, task.Session.Messages, index+1, index)
		snapshots = append(snapshots, task.Session)
		messages.appendCommitted(planStepRoundTrip(stepID)...)
		return generationresult.GenerationResponse{SessionID: task.Session.ID, Result: modelport.ChatResult{Content: stepID + " complete"}}, nil
	})
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
		Messages: messages, Generation: generation, MaxParallelSteps: 8,
	}
	result, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	if len(snapshots) != 2 || result.Plan == nil || result.Plan.Status != agentplan.StatusDone {
		t.Fatalf("expected both steps to finish: snapshots=%d plan=%#v", len(snapshots), result.Plan)
	}
	if result.Result.Content != "inspect complete\n\nverify complete" {
		t.Fatalf("step answers were lost or duplicated: %q", result.Result.Content)
	}
	latest := messages.snapshot()
	assertCompletePlanTranscript(t, latest.Messages, 2, 2)
	if len(snapshots[0].Messages) != 4 || len(snapshots[1].Messages) != 9 || len(latest.Messages) != 12 {
		t.Fatalf("generation snapshots must remain fixed: first=%d second=%d committed=%d", len(snapshots[0].Messages), len(snapshots[1].Messages), len(latest.Messages))
	}
	for index, snapshot := range snapshots {
		if !reflect.DeepEqual(snapshot.Messages, latest.Messages[:len(snapshot.Messages)]) {
			t.Fatalf("step %d snapshot was mutated by a later commit", index+1)
		}
	}
}

func TestPreparePlanStepUsesLatestPreparedSnapshot(t *testing.T) {
	current := &session.Session{ID: "session", Model: "old-model", PermissionMode: session.PermissionModeFull}
	current.AppendMessage(domainmessage.Text(domainmessage.RoleUser, "shared history"))
	baseline := session.Clone(current)
	latest := session.Clone(current)
	latest.Model = "latest-model"
	latest.PermissionMode = session.PermissionModeReadonly
	latest.AllowedTools = []string{"read_file"}
	latest.EnforceToolAllowlist = true
	latest.PermissionCeiling = []tooldef.Permission{tooldef.PermissionRead}
	latest.WorkspaceRoot = "latest-workspace"
	latest.WorkspaceSandboxID = "latest-sandbox"
	latest.MaxToolRounds = 3
	latest.AppendMessages(planStepRoundTrip("previous")...)
	messages := &appendingMessages{current: latest, runtimeInstruction: true}
	plan := &agentplan.Plan{ID: "plan", Steps: []agentplan.Step{{ID: "next", Order: 1, Title: "next", Status: agentplan.StepStatusRunning}}}
	service := ExecutionService{Messages: messages}
	task, err := service.preparePlanStep(context.Background(), current, baseline, plan, 0, false)
	if err != nil {
		t.Fatalf("prepare next step: %v", err)
	}
	if !reflect.DeepEqual(task.Session, messages.snapshot()) {
		t.Fatalf("prepared step reused stale baseline: got %#v, want %#v", task.Session, messages.snapshot())
	}
	assertCompletePlanTranscript(t, task.Session.Messages, 1, 1)
	if len(task.Session.Messages) != 6 || len(current.Messages) != 1 || len(baseline.Messages) != 1 {
		t.Fatalf("latest preparation duplicated a turn or changed old snapshots: task=%d current=%d baseline=%d", len(task.Session.Messages), len(current.Messages), len(baseline.Messages))
	}
	liveBeforeMutation := messages.snapshot()
	// A generation snapshot must also own its permission slices and tool parts.
	task.Session.AllowedTools[0] = "shell"
	task.Session.PermissionCeiling[0] = tooldef.PermissionExecute
	task.Session.Messages[1].Parts[1].ToolCall.Arguments = "changed by generator"
	task.Session.Messages[2].Parts[0].ToolResult.Content = "changed by generator"
	if got := messages.snapshot(); !reflect.DeepEqual(got, liveBeforeMutation) {
		t.Fatalf("generation snapshot mutated the live permission or tool graph: got %#v, want %#v", got, liveBeforeMutation)
	}
}

func TestReadyBatchStopsBeforePreparingNextStepOnFailureOrCancellation(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure error
	}{
		{name: "failure", failure: errors.New("step failed")},
		{name: "cancellation", failure: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := &session.Session{ID: "session", Model: "model"}
			messages := &appendingMessages{current: current}
			plan := &agentplan.Plan{ID: "plan", Steps: []agentplan.Step{{ID: "a", Title: "first"}, {ID: "b", Title: "second"}}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var generated []string
			service := ExecutionService{
				Messages: messages,
				Generation: planGenerationFunc(func(ctx context.Context, _ generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
					generated = append(generated, agentrunruntime.MetadataFrom(ctx).StepID)
					if errors.Is(test.failure, context.Canceled) {
						cancel()
					}
					return generationresult.GenerationResponse{}, test.failure
				}),
			}
			results := service.executeReadyBatch(ctx, current, plan, []int{0, 1}, "run", modelport.ChatStreamHandler{}, false)
			if len(results) != 1 || results[0].Index != 0 || !errors.Is(results[0].Err, test.failure) {
				t.Fatalf("batch failure was not preserved: %#v", results)
			}
			if !reflect.DeepEqual(generated, []string{"a"}) || len(messages.commands) != 1 || len(current.Messages) != 1 {
				t.Fatalf("batch prepared or ran a later step: generated=%v commands=%d messages=%d", generated, len(messages.commands), len(current.Messages))
			}
		})
	}
}

func planStepRoundTrip(stepID string) []domainmessage.Message {
	call := domainmessage.ToolCall{ID: "call-" + stepID, Type: "function", Name: "read_file", Arguments: fmt.Sprintf(`{"step":%q}`, stepID)}
	return []domainmessage.Message{
		domainmessage.Assistant(stepID+" checking", stepID+" reasoning", []domainmessage.ToolCall{call}),
		domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: stepID + " tool evidence"}),
		domainmessage.Text(domainmessage.RoleAssistant, stepID+" complete"),
	}
}

func assertCompletePlanTranscript(t *testing.T, messages []domainmessage.Message, wantSteps, wantRoundTrips int) {
	t.Helper()
	ids := make(map[string]bool)
	instructions := make(map[string]bool)
	runtimeInstructions := 0
	stepInstructions := 0
	calls := make(map[string]domainmessage.ToolCall)
	results := make(map[string]bool)
	answers := 0
	var previousSequence int64
	for index, message := range messages {
		if message.ID == "" || ids[message.ID] || message.Sequence <= previousSequence || message.CreatedAt.IsZero() {
			t.Fatalf("message %d lost unique ordered identity: %#v", index, message)
		}
		ids[message.ID] = true
		previousSequence = message.Sequence
		if message.IsSyntheticReason(domainmessage.SyntheticReasonRuntimeInstruction) || message.IsSyntheticReason(domainmessage.SyntheticReasonPlanStep) {
			if message.Role != domainmessage.RoleUser || instructions[message.Text()] {
				t.Fatalf("duplicate or incorrectly marked internal instruction at %d: %#v", index, message)
			}
			instructions[message.Text()] = true
			if message.IsSyntheticReason(domainmessage.SyntheticReasonRuntimeInstruction) {
				runtimeInstructions++
			} else {
				stepInstructions++
				if index == 0 || !messages[index-1].IsSyntheticReason(domainmessage.SyntheticReasonRuntimeInstruction) {
					t.Fatalf("plan step instruction %d lost its runtime instruction", index)
				}
			}
		}
		for _, part := range message.Parts {
			if part.ToolCall != nil {
				call := *part.ToolCall
				if message.Role != domainmessage.RoleAssistant || call.ID == "" || calls[call.ID].ID != "" {
					t.Fatalf("duplicate or orphaned assistant call at %d: %#v", index, message)
				}
				calls[call.ID] = call
				if message.Text() == "" || message.Reasoning == "" {
					t.Fatalf("assistant call lost its response text or reasoning at %d: %#v", index, message)
				}
			}
			if part.ToolResult != nil {
				result := *part.ToolResult
				call, ok := calls[result.ToolCallID]
				if message.Role != domainmessage.RoleTool || !ok || results[result.ToolCallID] || call.Name != result.Name || result.Content == "" {
					t.Fatalf("duplicate or orphaned tool result at %d: %#v", index, message)
				}
				results[result.ToolCallID] = true
			}
		}
		if message.Role == domainmessage.RoleAssistant && !message.HasToolCall() {
			if index == 0 || messages[index-1].Role != domainmessage.RoleTool || message.Text() == "" {
				t.Fatalf("step answer lost its preceding tool result at %d: %#v", index, message)
			}
			answers++
		}
	}
	if runtimeInstructions != wantSteps || stepInstructions != wantSteps || len(calls) != wantRoundTrips || len(results) != wantRoundTrips || answers != wantRoundTrips {
		t.Fatalf("transcript counts: runtime=%d step=%d calls=%d results=%d answers=%d, want steps=%d roundtrips=%d", runtimeInstructions, stepInstructions, len(calls), len(results), answers, wantSteps, wantRoundTrips)
	}
}
