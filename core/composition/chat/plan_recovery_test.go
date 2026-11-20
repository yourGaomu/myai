package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	generationadapter "myai/core/adapter/chat/generation"
	agentrunmemory "myai/core/adapter/persistence/memory/agentrun"
	outboxjournal "myai/core/adapter/persistence/outbox/journal"
	outboxrepository "myai/core/adapter/persistence/outbox/repository"
	memorysession "myai/core/adapter/session/memory"
	runtimeservice "myai/core/application/runtime/service"
	domainagentrun "myai/core/domain/agentrun"
	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	domainmodel "myai/core/domain/model"
	agentplan "myai/core/plan"
	historyport "myai/core/port/history"
	modelport "myai/core/port/model"
	outbox "myai/core/port/outbox"
	persistenceport "myai/core/port/persistence"
	repository "myai/core/port/repository"
	chatservice "myai/core/service"
	"myai/core/session"
	"myai/core/tool"
	tooldef "myai/core/tool/tool"
)

const (
	planRecoveryModelID      = "plan-recovery-model"
	planRecoveryToolName     = "plan_recovery_effect"
	planRecoveryInput        = "请实现项目中的两步回归功能"
	planRecoveryDraft        = `{"steps":[{"id":"first","title":"Apply first effect"},{"id":"second","title":"Apply second effect","depends_on":["first"]}]}`
	planRecoveryFirstAnswer  = "plan-recovery-first-finished"
	planRecoverySecondAnswer = "plan-recovery-second-finished"
)

func TestPlanRecoveryCompletedPlanSurvivesDelayedMessageQueueAndReload(t *testing.T) {
	h := newPlanRecoveryHarness(t)
	ctx := planRecoveryContext(t)
	h.plan(t, ctx)
	fullSaves := h.store.fullSaves
	response, err := h.chat.ExecutePlanStreamForSession(ctx, h.sessionID, modelport.ChatStreamHandler{}, nil)
	if err != nil {
		t.Fatalf("execute two-step plan through composition: %v", err)
	}
	if response.MessageID != "" {
		t.Fatalf("multi-step execution returned one reply ID: %q", response.MessageID)
	}
	assertPlanRecoveryDone(t, response.Plan)
	h.assertCalls(t, 5, 1, 1)

	// The real SessionQueue still owns every user/assistant/tool write. Plan CAS
	// has completed synchronously, so all queued snapshots are now stale.
	h.assertMessagesDelayed(t, ctx)
	before := h.persistedPlan(t, ctx)
	assertPlanRecoveryDone(t, before)
	if h.store.fullSaves != fullSaves || h.store.planSaves < 6 {
		t.Fatalf("unexpected persistence path: full saves %d -> %d, plan saves %d", fullSaves, h.store.fullSaves, h.store.planSaves)
	}
	live, err := h.memory.Snapshot(h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	h.flush(t, ctx)
	if after := h.persistedPlan(t, ctx); !reflect.DeepEqual(before, after) {
		t.Fatalf("delayed messages overwrote the completed plan: before=%+v after=%+v", before, after)
	}
	if h.store.messageTouches == 0 || h.store.fullSaves != fullSaves {
		t.Fatalf("message persistence did not use narrow metadata updates: touches=%d full saves=%d", h.store.messageTouches, h.store.fullSaves)
	}

	fresh := h.reload(t, ctx)
	loaded, err := fresh.memory.Snapshot(h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.CurrentPlan, before) {
		t.Fatalf("fresh Store loaded a different plan: got=%+v want=%+v", loaded.CurrentPlan, before)
	}
	assertPlanRecoveryTranscript(t, live.Messages, loaded.Messages)
	assertPlanRecoveryRoundTrips(t, loaded.Messages)
	assertPlanRecoveryAssistant(t, loaded.Messages, planRecoveryDraft)
	assertPlanRecoveryAssistant(t, loaded.Messages, planRecoveryFirstAnswer)
	assertPlanRecoveryAssistant(t, loaded.Messages, planRecoverySecondAnswer)

	if _, err := fresh.chat.ExecutePlanStreamForSession(ctx, h.sessionID, modelport.ChatStreamHandler{}, nil); err == nil {
		t.Fatal("executing a reloaded completed plan must be rejected")
	}
	fresh.assertCalls(t, 5, 1, 1)
	if after := fresh.persistedPlan(t, ctx); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected execution changed the persisted plan: %+v", after)
	}
	t.Logf("two tool effects, 5 model calls, %d durable message records, completed revision %d survives late queue drain and cold reload", len(h.store.messageRecords(h.sessionID)), before.Revision)
}

func TestPlanRecoveryPlanOnlyDraftReloadsWithoutChangingAgentMode(t *testing.T) {
	h := newPlanRecoveryHarness(t)
	ctx := planRecoveryContext(t)
	response := h.plan(t, ctx)
	h.assertCalls(t, 1, 0, 0)
	h.assertMessagesDelayed(t, ctx)
	live, err := h.memory.Snapshot(h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	answer := assertPlanRecoveryAssistant(t, live.Messages, planRecoveryDraft)
	if response.MessageID != answer.ID {
		t.Fatalf("single draft reply ID = %q, want %q", response.MessageID, answer.ID)
	}
	if live.AgentMode != session.AgentModeChat || response.Plan.Status != agentplan.StatusDraft {
		t.Fatalf("planning changed the durable mode or executed the draft: mode=%q plan=%+v", live.AgentMode, response.Plan)
	}
	requests := h.model.snapshotRequests()
	if len(requests[0].Tools) != 0 {
		t.Fatalf("write tool was offered before plan confirmation: %+v", requests[0].Tools)
	}
	h.flush(t, ctx)

	fresh := h.reload(t, ctx)
	loaded, err := fresh.memory.Snapshot(h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AgentMode != session.AgentModeChat || !reflect.DeepEqual(loaded.CurrentPlan, response.Plan) {
		t.Fatalf("draft/mode did not survive cold reload: mode=%q plan=%+v", loaded.AgentMode, loaded.CurrentPlan)
	}
	assertPlanRecoverySameStamp(t, answer, assertPlanRecoveryAssistant(t, loaded.Messages, planRecoveryDraft))
	assertPlanRecoveryTranscript(t, live.Messages, loaded.Messages)
	fresh.assertCalls(t, 1, 0, 0)

	// A normal explicit confirmation traverses classification, message append,
	// execution, tools, and persistence instead of invoking a fake plan runner.
	executed, err := fresh.chat.SendMessageStreamForSession(ctx, h.sessionID, "执行计划", modelport.ChatStreamHandler{})
	if err != nil {
		t.Fatalf("explicitly confirm the reloaded draft: %v", err)
	}
	assertPlanRecoveryDone(t, executed.Plan)
	if executed.MessageID != "" {
		t.Fatalf("explicit chat confirmation attached a step ID to the plan aggregate: %q", executed.MessageID)
	}
	fresh.assertCalls(t, 5, 1, 1)
	fresh.flush(t, ctx)
	confirmed := fresh.reload(t, ctx)
	confirmedState, err := confirmed.memory.Snapshot(h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	assertPlanRecoveryDone(t, confirmedState.CurrentPlan)
	assertPlanRecoveryRoundTrips(t, confirmedState.Messages)
	if confirmedState.AgentMode != session.AgentModeChat {
		t.Fatalf("explicit plan execution changed AgentMode: %q", confirmedState.AgentMode)
	}
	t.Logf("draft answer %s/%d survives reload in chat mode; tool counts stay 0 until explicit confirmation, then each step executes once", answer.ID, answer.Sequence)
}

func TestPlanRecoveryRunningPlanReloadDoesNotReplayEffects(t *testing.T) {
	testPlanRecoveryInterrupted(t, agentplan.StatusRunning)
}

func TestPlanRecoveryRunningStepReloadDoesNotReplayEffects(t *testing.T) {
	// A legacy/non-running aggregate status must not hide an interrupted step.
	testPlanRecoveryInterrupted(t, agentplan.StatusFailed)
}

func testPlanRecoveryInterrupted(t *testing.T, persistedStatus string) {
	t.Helper()
	h := newPlanRecoveryHarness(t)
	ctx := planRecoveryContext(t)
	h.plan(t, ctx)
	reached := make(chan struct{})
	h.model.beforeSample = func(ctx context.Context, index int) error {
		if index != 2 {
			return nil
		}
		// The first effect and its tool result have committed to the real memory
		// Store, while the final model answer has not returned to the executor.
		close(reached)
		<-ctx.Done()
		return ctx.Err()
	}
	runCtx, cancel := context.WithCancel(ctx)
	finished := make(chan error, 1)
	go func() {
		_, err := h.chat.ExecutePlanStreamForSession(runCtx, h.sessionID, modelport.ChatStreamHandler{}, nil)
		finished <- err
	}()
	defer func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("interrupted execution did not stop after cancellation")
		}
	}()
	select {
	case <-reached:
	case err := <-finished:
		// Leave the result available for deferred cleanup as well.
		finished <- err
		t.Fatalf("execution stopped before its first side effect: %v", err)
	case <-ctx.Done():
		t.Fatal("execution did not reach the first completed tool effect")
	}
	h.assertCalls(t, 3, 1, 0)
	h.flush(t, ctx)
	interrupted := h.persistedPlan(t, ctx)
	if interrupted.Status != agentplan.StatusRunning || interrupted.Steps[0].Status != agentplan.StepStatusRunning {
		t.Fatalf("expected a real interrupted step, got %+v", interrupted)
	}
	if persistedStatus != interrupted.Status {
		next := agentplan.Clone(interrupted)
		next.Status, next.Revision = persistedStatus, interrupted.Revision+1
		if err := h.store.SaveSessionPlan(ctx, h.sessionID, interrupted.ID, interrupted.Revision, next); err != nil {
			t.Fatalf("persist legacy aggregate status with a running step: %v", err)
		}
		interrupted = next
	}

	fresh := h.reload(t, ctx)
	loaded, err := fresh.memory.Snapshot(h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.CurrentPlan, interrupted) {
		t.Fatalf("fresh Store lost the interrupted plan: %+v", loaded.CurrentPlan)
	}
	foundEffect := false
	for _, message := range loaded.Messages {
		if result, ok := message.FirstToolResult(); ok && result.ToolCallID == "effect-first" && result.Content == "effect:first" {
			foundEffect = true
		}
	}
	if !foundEffect {
		t.Fatal("cold reload lost the already applied first tool effect")
	}
	fresh.assertCalls(t, 3, 1, 0)
	if _, err := fresh.chat.ExecutePlanStreamForSession(ctx, h.sessionID, modelport.ChatStreamHandler{}, nil); err == nil || !strings.Contains(err.Error(), "interrupted running") {
		t.Fatalf("reloaded interrupted execution must require resolution, got %v", err)
	}
	if _, err := fresh.chat.SendMessageStreamForSession(ctx, h.sessionID, "继续", modelport.ChatStreamHandler{}); err == nil || !strings.Contains(err.Error(), "interrupted running") {
		t.Fatalf("continuation must not replay an interrupted effect, got %v", err)
	}
	fresh.assertCalls(t, 3, 1, 0)
	if after := fresh.persistedPlan(t, ctx); !reflect.DeepEqual(after, interrupted) {
		t.Fatalf("refused recovery mutated the persisted plan: %+v", after)
	}
	t.Logf("cold reload of plan status %s with running first step preserves its durable tool result and refuses replay at model/tool counts 3/1", persistedStatus)
}

func TestPlanRecoveryPlanCASFailureStillPersistsAnswerAndRunEvent(t *testing.T) {
	h := newPlanRecoveryHarness(t)
	ctx := planRecoveryContext(t)
	h.store.planErr = repository.ErrConflict
	response, err := h.chat.SendMessageStreamForSession(ctx, h.sessionID, planRecoveryInput, modelport.ChatStreamHandler{})
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("plan persistence error = %v, want ErrConflict", err)
	}
	if response.Result.Content != planRecoveryDraft || response.Plan != nil || response.RunID == "" {
		t.Fatalf("CAS failure lost successful model answer/run identity: %+v", response)
	}
	h.assertCalls(t, 1, 0, 0)
	live, err := h.memory.Snapshot(h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	answer := assertPlanRecoveryAssistant(t, live.Messages, planRecoveryDraft)
	if live.CurrentPlan != nil {
		t.Fatalf("failed plan CAS left a live plan: %+v", live.CurrentPlan)
	}
	events, err := h.runs.ListEvents(ctx, []string{response.RunID})
	if err != nil {
		t.Fatal(err)
	}
	foundAnswer := false
	for _, event := range events {
		if event.Type == domainagentrun.EventTypeAnswer && event.Content == planRecoveryDraft {
			foundAnswer = true
		}
	}
	if !foundAnswer {
		t.Fatalf("CAS failure discarded the AgentRun answer: %+v", events)
	}
	run, err := h.runs.GetRun(ctx, response.RunID)
	if err != nil || run.Status != domainagentrun.StatusFailed {
		t.Fatalf("plan CAS failure must remain visible in the run: run=%+v err=%v", run, err)
	}
	h.assertMessagesDelayed(t, ctx)
	h.flush(t, ctx)
	fresh := h.reload(t, ctx)
	loaded, err := fresh.memory.Snapshot(h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	assertPlanRecoverySameStamp(t, answer, assertPlanRecoveryAssistant(t, loaded.Messages, planRecoveryDraft))
	assertPlanRecoveryTranscript(t, live.Messages, loaded.Messages)
	if loaded.CurrentPlan != nil || loaded.AgentMode != session.AgentModeChat {
		t.Fatalf("CAS failure reappeared as a plan or changed mode after reload: %+v", loaded)
	}
	fresh.assertCalls(t, 1, 0, 0)
	t.Logf("ErrConflict preserves response, AgentRun answer event, and cold-loaded assistant identity %s/%d", answer.ID, answer.Sequence)
}

type replyIdentityModel struct{}

type replyIdentityInlineExecutor struct{}

func (replyIdentityInlineExecutor) Submit(task func()) error {
	task()
	return nil
}

func (replyIdentityModel) Generate(_ context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	const reply = "same answer"
	if request.Stream.OnAnswer != nil {
		request.Stream.OnAnswer(reply)
	}
	return modelport.ChatResult{Content: reply}, nil
}

func TestAssistantReplyIDSurvivesDelayedPersistenceAndRegeneration(t *testing.T) {
	h := newPlanRecoveryHarness(t)
	ctx := planRecoveryContext(t)
	configuration := h.configuration(t)
	configuration.Models = &planRecoveryRegistry{model: replyIdentityModel{}}
	h.chat = NewService(configuration)

	send := func(input string) chatservice.ChatResponse {
		t.Helper()
		response, err := h.chat.SendMessageStreamForSession(ctx, h.sessionID, input, modelport.ChatStreamHandler{})
		if err != nil || response.MessageID == "" || response.Result.Content != "same answer" {
			t.Fatalf("send %q did not return a committed reply: response=%+v error=%v", input, response, err)
		}
		return response
	}
	assertHistory := func(chat *chatservice.ChatService, want ...string) {
		t.Helper()
		messages, err := chat.ListSessionMessages(ctx, h.sessionID)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, message := range messages {
			if message.Role == repository.RoleAssistant {
				ids = append(ids, message.ID)
			}
		}
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("assistant history IDs = %v, want %v", ids, want)
		}
	}
	first := send("First question?")
	h.flush(t, ctx)
	second := send("Second question?")
	if second.MessageID == first.MessageID {
		t.Fatal("two replies with the same body share an ID")
	}
	assertHistory(h.chat, first.MessageID, second.MessageID)
	meta, err := h.chat.SessionHistoryMeta(ctx, h.sessionID)
	if err != nil || meta.LastMessageID != second.MessageID {
		t.Fatalf("pending reply absent from history metadata: %+v error=%v", meta, err)
	}
	delta, fullSync, err := h.chat.ListSessionMessagesAfter(ctx, h.sessionID, first.MessageID, 10)
	if err != nil || fullSync || len(delta) != 2 || delta[1].ID != second.MessageID {
		t.Fatalf("pending reply absent from history delta: %+v fullSync=%t error=%v", delta, fullSync, err)
	}
	for _, record := range h.store.messageRecords(h.sessionID) {
		if record.ID == second.MessageID {
			t.Fatal("second reply should still be pending database persistence")
		}
	}
	h.flush(t, ctx)
	assertHistory(h.reload(t, ctx).chat, first.MessageID, second.MessageID)

	// Regeneration waits for the transcript replacement before sampling. Use
	// the caller-runs executor so this phase can perform its synchronous write.
	configuration.PersistenceQueue = nil
	configuration.Async = replyIdentityInlineExecutor{}
	h.chat = NewService(configuration)
	regenerated, err := h.chat.RegenerateLastMessageStreamForSession(ctx, h.sessionID, modelport.ChatStreamHandler{})
	if err != nil || regenerated.MessageID == "" || regenerated.MessageID == first.MessageID || regenerated.MessageID == second.MessageID {
		t.Fatalf("regeneration did not get a new committed reply ID: response=%+v error=%v", regenerated, err)
	}
	assertHistory(h.chat, first.MessageID, regenerated.MessageID)
	h.flush(t, ctx)
	assertHistory(h.reload(t, ctx).chat, first.MessageID, regenerated.MessageID)

	for _, response := range []chatservice.ChatResponse{first, second, regenerated} {
		events, err := h.runs.ListEvents(ctx, []string{response.RunID})
		if err != nil {
			t.Fatal(err)
		}
		var answers []string
		for _, event := range events {
			if event.Type == domainagentrun.EventTypeAnswer {
				answers = append(answers, event.MessageID)
			}
		}
		if !reflect.DeepEqual(answers, []string{response.MessageID}) {
			t.Fatalf("answer event IDs = %v, want %s for run %s", answers, response.MessageID, response.RunID)
		}
	}
}

type planRecoveryHarness struct {
	chat      *chatservice.ChatService
	memory    *memorysession.Store
	store     *planRecoveryStore
	async     *planRecoveryDelayedExecutor
	model     *planRecoveryModel
	effect    *planRecoveryEffectTool
	tools     *tool.RegisterTools
	runs      *agentrunmemory.Repository
	sessionID string
	queue     *generationadapter.SessionQueue
}

func newPlanRecoveryHarness(t *testing.T) *planRecoveryHarness {
	t.Helper()
	h := &planRecoveryHarness{
		memory: memorysession.NewStore(planRecoveryModelID),
		store:  &planRecoveryStore{sessions: make(map[string]repository.SessionRecord), messages: make(map[string]repository.MessageRecord)},
		async:  &planRecoveryDelayedExecutor{}, model: &planRecoveryModel{},
		effect: &planRecoveryEffectTool{calls: make(map[string]int)}, tools: tool.NewRegisterTools(), runs: agentrunmemory.New(),
	}
	h.tools.Register(h.effect)
	h.chat = NewService(h.configuration(t))
	ctx := planRecoveryContext(t)
	var err error
	h.sessionID, err = h.chat.CreateSession(ctx, "Plan recovery regression")
	if err != nil {
		t.Fatalf("create session through composition: %v", err)
	}
	if err := h.chat.SetPermissionModeForSession(ctx, h.sessionID, string(session.PermissionModeFull)); err != nil {
		t.Fatalf("set in-memory fake tool permission: %v", err)
	}
	return h
}

func (h *planRecoveryHarness) configuration(t *testing.T) Configuration {
	t.Helper()
	journal, err := outboxjournal.Open(filepath.Join(t.TempDir(), "journal", "pending.db"), outboxjournal.Options{
		Scope: outbox.Hash([]byte("plan-recovery-fixture")),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	h.queue, err = generationadapter.NewDurableSessionQueue(
		runtimeservice.AsyncTaskService{Executor: h.async}, journal,
		outboxrepository.Writer{Messages: h.store, Assets: h.store, Metadata: h.store, Sessions: h.store},
	)
	if err != nil {
		t.Fatal(err)
	}
	return Configuration{
		Sessions: h.memory, Store: h.store, Async: h.async, DefaultModel: planRecoveryModelID,
		Models: &planRecoveryRegistry{model: h.model}, Tools: h.tools, AgentRuns: h.runs,
		HistoryStoreFactory: planRecoveryNoHistory{t: t}, PersistenceQueue: h.queue,
	}
}

func (h *planRecoveryHarness) plan(t *testing.T, ctx context.Context) chatservice.ChatResponse {
	t.Helper()
	response, err := h.chat.SendMessageStreamForSession(ctx, h.sessionID, planRecoveryInput, modelport.ChatStreamHandler{})
	if err != nil {
		t.Fatalf("generate plan_only draft through composition: %v", err)
	}
	if response.Result.Content != planRecoveryDraft || response.Plan == nil || response.Plan.Status != agentplan.StatusDraft || len(response.Plan.Steps) != 2 {
		t.Fatalf("expected a persisted, unexecuted two-step draft, got %+v", response)
	}
	h.assertCalls(t, 1, 0, 0)
	return response
}

func (h *planRecoveryHarness) reload(t *testing.T, ctx context.Context) *planRecoveryHarness {
	t.Helper()
	fresh := *h
	fresh.memory = memorysession.NewStore(planRecoveryModelID)
	fresh.async = &planRecoveryDelayedExecutor{}
	if _, err := fresh.memory.GetSession(h.sessionID); err == nil {
		t.Fatal("reload fixture unexpectedly contains the original live session")
	}
	fresh.chat = NewService(fresh.configuration(t))
	if err := fresh.chat.LoadSession(ctx, h.sessionID); err != nil {
		t.Fatalf("load persisted metadata and messages into a fresh Store: %v", err)
	}
	return &fresh
}

func (h *planRecoveryHarness) persistedPlan(t *testing.T, ctx context.Context) *agentplan.Plan {
	t.Helper()
	record, err := h.store.GetSession(ctx, h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return record.CurrentPlan
}

func (h *planRecoveryHarness) assertMessagesDelayed(t *testing.T, ctx context.Context) {
	t.Helper()
	messages, err := h.store.ListMessages(ctx, h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 || h.async.pending() == 0 {
		t.Fatalf("message queue was not actually delayed: durable messages=%d scheduled drains=%d", len(messages), h.async.pending())
	}
	stats, err := h.queue.Stats(ctx)
	if err != nil || stats.Pending == 0 {
		t.Fatalf("delayed records were not durably accepted: stats=%+v error=%v", stats, err)
	}
}

func (h *planRecoveryHarness) flush(t *testing.T, ctx context.Context) {
	t.Helper()
	h.async.drain()
	if err := h.chat.FlushPersistence(ctx); err != nil {
		t.Fatalf("drain the real composition persistence queue: %v", err)
	}
}

func (h *planRecoveryHarness) assertCalls(t *testing.T, modelCalls, first, second int) {
	t.Helper()
	if got := len(h.model.snapshotRequests()); got != modelCalls {
		t.Fatalf("model calls = %d, want %d", got, modelCalls)
	}
	h.effect.mu.Lock()
	defer h.effect.mu.Unlock()
	if h.effect.calls["first"] != first || h.effect.calls["second"] != second || len(h.effect.calls) > 2 {
		t.Fatalf("tool effects = %v, want first=%d second=%d", h.effect.calls, first, second)
	}
}

func planRecoveryContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func assertPlanRecoveryDone(t *testing.T, current *agentplan.Plan) {
	t.Helper()
	if current == nil || current.Status != agentplan.StatusDone || len(current.Steps) != 2 {
		t.Fatalf("expected completed two-step plan, got %+v", current)
	}
	for _, step := range current.Steps {
		if step.Status != agentplan.StepStatusDone {
			t.Fatalf("completed plan contains unfinished step: %+v", step)
		}
	}
}

func assertPlanRecoveryAssistant(t *testing.T, messages []domainmessage.Message, content string) domainmessage.Message {
	t.Helper()
	var found []domainmessage.Message
	for _, message := range messages {
		if message.Role == domainmessage.RoleAssistant && message.Text() == content {
			found = append(found, message)
		}
	}
	if len(found) != 1 {
		t.Fatalf("assistant answer %q occurs %d times, want exactly once", content, len(found))
	}
	if found[0].ID == "" || found[0].Sequence <= 0 || found[0].CreatedAt.IsZero() {
		t.Fatalf("assistant answer has no stable identity: %+v", found[0])
	}
	return found[0]
}

func assertPlanRecoverySameStamp(t *testing.T, want, got domainmessage.Message) {
	t.Helper()
	if got.ID != want.ID || got.Sequence != want.Sequence || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("message identity changed across persistence: got=%s/%d/%s want=%s/%d/%s", got.ID, got.Sequence, got.CreatedAt, want.ID, want.Sequence, want.CreatedAt)
	}
}

func assertPlanRecoveryTranscript(t *testing.T, want, got []domainmessage.Message) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cold transcript has %d messages, live transcript has %d", len(got), len(want))
	}
	// The fixed system prompt is rebuilt; every persisted message must keep its
	// stamp, content, tool graph, and provenance. RecordIDs are storage cursors.
	for index := 1; index < len(want); index++ {
		assertPlanRecoverySameStamp(t, want[index], got[index])
		left, right := domainmessage.Clone(want[index]), domainmessage.Clone(got[index])
		left.RecordIDs, right.RecordIDs = nil, nil
		if original, ok := left.FirstToolResult(); ok {
			loaded, exists := right.FirstToolResult()
			if !exists || original.PromptContent() != loaded.PromptContent() {
				t.Fatalf("cold tool prompt changed: got=%+v want=%+v", loaded, original)
			}
			if !reflect.DeepEqual(original, loaded) {
				t.Logf("tool result before canonicalizing untruncated full-text defaults: live=%+v loaded=%+v", original, loaded)
			}
		}
		// The load mapper populates redundant full text even for an untruncated
		// result. Fill only empty defaults; retain all nonempty full text and
		// truncation flags so content loss still fails the complete comparison.
		for _, message := range []*domainmessage.Message{&left, &right} {
			for _, part := range message.Parts {
				if result := part.ToolResult; result != nil && !result.PromptTruncated {
					if result.FullContent == "" {
						result.FullContent = result.Content
					}
					if result.FullErrorMessage == "" {
						result.FullErrorMessage = result.ErrorMessage
					}
				}
			}
		}
		if !reflect.DeepEqual(left, right) {
			leftJSON, _ := json.Marshal(left)
			rightJSON, _ := json.Marshal(right)
			t.Fatalf("cold transcript message %d differs: got=%s want=%s", index, rightJSON, leftJSON)
		}
	}
}

func assertPlanRecoveryRoundTrips(t *testing.T, messages []domainmessage.Message) {
	t.Helper()
	calls, results := make(map[string]int), make(map[string]int)
	for _, message := range messages {
		for _, part := range message.Parts {
			if part.ToolCall != nil {
				calls[part.ToolCall.ID]++
			}
			if part.ToolResult != nil {
				results[part.ToolResult.ToolCallID]++
				if part.ToolResult.Name != planRecoveryToolName || part.ToolResult.ErrorMessage != "" {
					t.Fatalf("tool effect did not succeed: %+v", part.ToolResult)
				}
			}
		}
	}
	want := map[string]int{"effect-first": 1, "effect-second": 1}
	if !reflect.DeepEqual(calls, want) || !reflect.DeepEqual(results, want) {
		t.Fatalf("cold reload lost or duplicated tool round trips: calls=%v results=%v", calls, results)
	}
}

type planRecoveryDelayedExecutor struct {
	mu    sync.Mutex
	tasks []func()
}

func (e *planRecoveryDelayedExecutor) Submit(task func()) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tasks = append(e.tasks, task)
	return nil
}

func (e *planRecoveryDelayedExecutor) pending() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.tasks)
}

func (e *planRecoveryDelayedExecutor) drain() {
	for {
		e.mu.Lock()
		if len(e.tasks) == 0 {
			e.mu.Unlock()
			return
		}
		task := e.tasks[0]
		e.tasks = e.tasks[1:]
		e.mu.Unlock()
		task()
	}
}

type planRecoveryModel struct {
	mu           sync.Mutex
	requests     []modelport.GenerateRequest
	beforeSample func(context.Context, int) error
}

func (m *planRecoveryModel) Generate(ctx context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	m.mu.Lock()
	index := len(m.requests)
	request.Messages = domainmessage.CloneAll(request.Messages)
	request.Tools = append([]modelport.Tool(nil), request.Tools...)
	m.requests = append(m.requests, request)
	before := m.beforeSample
	m.mu.Unlock()
	if before != nil {
		if err := before(ctx, index); err != nil {
			return modelport.ChatResult{}, err
		}
	}
	var result modelport.ChatResult
	switch index {
	case 0:
		result.Content = planRecoveryDraft
	case 1, 3:
		step := "first"
		if index == 3 {
			step = "second"
			if !planRecoveryRequestHasAnswer(request, planRecoveryFirstAnswer) {
				return modelport.ChatResult{}, errors.New("second step did not receive the committed first answer")
			}
		}
		found := false
		for _, candidate := range request.Tools {
			found = found || candidate.Function != nil && candidate.Function.Name == planRecoveryToolName
		}
		if !found {
			return modelport.ChatResult{}, errors.New("confirmed plan step has no effect tool")
		}
		result.ToolCalls = []modelport.ToolCall{{ID: "effect-" + step, Type: "function", Name: planRecoveryToolName, Arguments: fmt.Sprintf(`{"step":%q}`, step)}}
	case 2, 4:
		step := "first"
		result.Content = planRecoveryFirstAnswer
		if index == 4 {
			step, result.Content = "second", planRecoverySecondAnswer
		}
		found := false
		for _, message := range request.Messages {
			if effect, ok := message.FirstToolResult(); ok && effect.ToolCallID == "effect-"+step && effect.Content == "effect:"+step {
				found = true
			}
		}
		if !found {
			return modelport.ChatResult{}, fmt.Errorf("model did not receive completed %s effect", step)
		}
	default:
		return modelport.ChatResult{}, fmt.Errorf("unexpected model call %d: a completed step may have replayed", index+1)
	}
	if result.Content != "" && request.Stream.OnAnswer != nil {
		request.Stream.OnAnswer(result.Content)
	}
	return result, nil
}

func planRecoveryRequestHasAnswer(request modelport.GenerateRequest, content string) bool {
	for _, message := range request.Messages {
		if message.Role == domainmessage.RoleAssistant && message.Text() == content {
			return true
		}
	}
	return false
}

func (m *planRecoveryModel) snapshotRequests() []modelport.GenerateRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]modelport.GenerateRequest(nil), m.requests...)
}

type planRecoveryRegistry struct{ model modelport.ChatModelPort }

func (r *planRecoveryRegistry) GetModel(id string) modelport.ChatModelPort {
	if id == planRecoveryModelID {
		return r.model
	}
	return nil
}
func (r *planRecoveryRegistry) HasModel(id string) bool { return r.GetModel(id) != nil }
func (r *planRecoveryRegistry) ListModels() []modelport.ModelInfo {
	return []modelport.ModelInfo{{ID: planRecoveryModelID, ContextWindowTokens: 128000}}
}
func (r *planRecoveryRegistry) GetModelInfo(id string) (modelport.ModelInfo, bool) {
	return r.ListModels()[0], r.HasModel(id)
}
func (r *planRecoveryRegistry) SetModelInfo(_ string, model modelport.ChatModelPort, _ modelport.ModelInfo) {
	r.model = model
}
func (r *planRecoveryRegistry) RemoveModel(id string) bool {
	found := r.HasModel(id)
	if found {
		r.model = nil
	}
	return found
}

type planRecoveryEffectTool struct {
	mu    sync.Mutex
	calls map[string]int
}

func (*planRecoveryEffectTool) Name() string { return planRecoveryToolName }
func (*planRecoveryEffectTool) Description() string {
	return "Record a test-only side effect in memory"
}
func (*planRecoveryEffectTool) Permission() tooldef.Permission { return tooldef.PermissionWrite }
func (*planRecoveryEffectTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{"step": map[string]any{"type": "string"}}, "required": []string{"step"}, "additionalProperties": false}
}
func (e *planRecoveryEffectTool) Call(_ context.Context, raw json.RawMessage) (tooldef.ToolOutput, error) {
	var args struct {
		Step string `json:"step"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return tooldef.ToolOutput{}, err
	}
	if args.Step != "first" && args.Step != "second" {
		return tooldef.ToolOutput{}, errors.New("unexpected test effect")
	}
	e.mu.Lock()
	e.calls[args.Step]++
	e.mu.Unlock()
	return tooldef.SuccessOutput("effect:" + args.Step), nil
}

// Fail closed if composition ever attempts to open real history storage. The
// fake tool only changes a counter, so no workspace recorder should be needed.
type planRecoveryNoHistory struct{ t *testing.T }

func (f planRecoveryNoHistory) Open(string) (historyport.Store, error) {
	f.t.Error("plan recovery test attempted to open history storage")
	return nil, errors.New("history storage forbidden in plan recovery test")
}
func (f planRecoveryNoHistory) DefaultPath(string) (string, error) {
	f.t.Error("plan recovery test attempted to resolve a historydb path")
	return "", errors.New("history storage forbidden in plan recovery test")
}

// This is a durable-record fake, not a Session loader or monotonic Plan filter.
// SaveSession unconditionally replaces records, so a stale whole-session write
// would regress the plan. Only the production narrow interfaces own Plan CAS
// and message metadata; SaveMessage enforces immutable, stable record IDs.
type planRecoveryStore struct {
	mu             sync.Mutex
	sessions       map[string]repository.SessionRecord
	messages       map[string]repository.MessageRecord
	assets         map[string]repository.AssetRecord
	configs        map[string]domainmodel.Config
	usageSequence  map[string]int64
	fullSaves      int
	messageTouches int
	planSaves      int
	planErr        error
}

var (
	_ persistenceport.Store                  = (*planRecoveryStore)(nil)
	_ repository.SessionMessageMetadataStore = (*planRecoveryStore)(nil)
	_ repository.SessionPlanStore            = (*planRecoveryStore)(nil)
	_ ModelRegistry                          = (*planRecoveryRegistry)(nil)
)

func (s *planRecoveryStore) GetSession(_ context.Context, id string) (repository.SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.sessions[id]
	if !ok {
		return repository.SessionRecord{}, repository.ErrNotFound
	}
	return clonePlanRecoveryRecord(record), nil
}
func (s *planRecoveryStore) SaveSession(_ context.Context, record repository.SessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fullSaves++
	s.sessions[record.ID] = clonePlanRecoveryRecord(record)
	return nil
}
func (s *planRecoveryStore) TouchSessionMessages(_ context.Context, record repository.SessionRecord, sequence int64, includeUsage bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageTouches++
	current, ok := s.sessions[record.ID]
	if !ok {
		current = clonePlanRecoveryRecord(record)
		current.CurrentPlan, current.AgentMode = nil, string(session.AgentModeChat)
		current.Usage, current.LastUsage = nil, nil
	}
	if record.UpdatedAt.After(current.UpdatedAt) {
		current.UpdatedAt = record.UpdatedAt
	}
	if current.Title == "" || current.Title == "New chat" {
		current.Title = record.Title
	}
	if s.usageSequence == nil {
		s.usageSequence = make(map[string]int64)
	}
	if includeUsage && sequence > s.usageSequence[record.ID] {
		current.Usage, current.LastUsage = clonePlanRecoveryUsage(record.Usage), clonePlanRecoveryUsage(record.LastUsage)
		s.usageSequence[record.ID] = sequence
	}
	s.sessions[record.ID] = current
	return nil
}
func (s *planRecoveryStore) SaveSessionPlan(_ context.Context, id, expectedID string, revision int64, next *agentplan.Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planSaves++
	if s.planErr != nil {
		return s.planErr
	}
	current, ok := s.sessions[id]
	if !ok {
		return repository.ErrNotFound
	}
	actualID, actualRevision := "", int64(0)
	if current.CurrentPlan != nil {
		actualID, actualRevision = current.CurrentPlan.ID, current.CurrentPlan.Revision
	}
	if actualID != expectedID || actualRevision != revision {
		if reflect.DeepEqual(current.CurrentPlan, next) {
			return nil
		}
		return repository.ErrConflict
	}
	if next != nil && (next.ID == "" || next.SessionID != id || (next.ID == expectedID && next.Revision != revision+1) || (next.ID != expectedID && next.Revision != 1)) {
		return repository.ErrConflict
	}
	current.CurrentPlan = agentplan.Clone(next)
	s.sessions[id] = current
	return nil
}
func (s *planRecoveryStore) SaveMessage(_ context.Context, record repository.MessageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record.ID == "" || record.SessionID == "" || record.Sequence <= 0 || record.CreatedAt.IsZero() {
		return errors.New("message has no stable identity")
	}
	if existing, ok := s.messages[record.ID]; ok && !reflect.DeepEqual(existing, record) {
		return repository.ErrConflict
	}
	s.messages[record.ID] = record
	return nil
}
func (s *planRecoveryStore) messageRecords(id string) []repository.MessageRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []repository.MessageRecord
	for _, record := range s.messages {
		if record.SessionID == id {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Sequence != records[j].Sequence {
			return records[i].Sequence < records[j].Sequence
		}
		if !records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].CreatedAt.Before(records[j].CreatedAt)
		}
		return records[i].ID < records[j].ID
	})
	return records
}
func (s *planRecoveryStore) ListMessages(_ context.Context, id string) ([]repository.MessageRecord, error) {
	return s.messageRecords(id), nil
}
func (s *planRecoveryStore) ClearMessages(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, record := range s.messages {
		if record.SessionID == id {
			delete(s.messages, key)
		}
	}
	return nil
}
func (s *planRecoveryStore) GetMessageHistoryMeta(_ context.Context, id string) (repository.MessageHistoryMeta, error) {
	meta := repository.MessageHistoryMeta{SessionID: id}
	for _, record := range s.messageRecords(id) {
		if record.SyntheticReason != "" {
			continue
		}
		meta.MessageCount++
		meta.LastMessageID, meta.HistoryVersion = record.ID, record.Sequence
		at := record.CreatedAt
		meta.LastMessageCreatedAt = &at
	}
	return meta, nil
}
func (s *planRecoveryStore) ListMessagesAfter(_ context.Context, id, after string, limit int) ([]repository.MessageRecord, bool, error) {
	var visible []repository.MessageRecord
	found := after == ""
	for _, record := range s.messageRecords(id) {
		if record.SyntheticReason != "" {
			continue
		}
		if !found {
			found = record.ID == after
			continue
		}
		visible = append(visible, record)
	}
	if limit > 0 && len(visible) > limit {
		return visible[:limit], true, nil
	}
	return visible, false, nil
}
func (s *planRecoveryStore) MarkSessionDeleted(ctx context.Context, id string, at time.Time) error {
	return s.setDeleted(ctx, id, true, at)
}
func (s *planRecoveryStore) MarkSessionRestored(ctx context.Context, id string, at time.Time) error {
	return s.setDeleted(ctx, id, false, at)
}
func (s *planRecoveryStore) setDeleted(_ context.Context, id string, deleted bool, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.sessions[id]
	if !ok {
		return repository.ErrNotFound
	}
	record.Deleted, record.DeletedAt, record.UpdatedAt = deleted, nil, at
	if deleted {
		record.DeletedAt = &at
	}
	s.sessions[id] = record
	return nil
}
func (s *planRecoveryStore) ListSessions(ctx context.Context) ([]repository.SessionRecord, error) {
	return s.ListSessionsWithDeleted(ctx, false)
}
func (s *planRecoveryStore) ListSessionsWithDeleted(_ context.Context, include bool) ([]repository.SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []repository.SessionRecord
	for _, record := range s.sessions {
		if !record.Deleted || include {
			records = append(records, clonePlanRecoveryRecord(record))
		}
	}
	return records, nil
}
func (s *planRecoveryStore) SaveAsset(_ context.Context, record repository.AssetRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.assets == nil {
		s.assets = make(map[string]repository.AssetRecord)
	}
	if existing, ok := s.assets[record.ID]; ok && !reflect.DeepEqual(existing, record) {
		return repository.ErrConflict
	}
	s.assets[record.ID] = record
	return nil
}
func (s *planRecoveryStore) ListAssets(_ context.Context, id string, limit int) ([]repository.AssetRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []repository.AssetRecord
	for _, record := range s.assets {
		if record.SessionID == id {
			records = append(records, record)
		}
	}
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}
func (s *planRecoveryStore) ReplaceSessionTranscript(_ context.Context, snapshot repository.TranscriptSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, record := range s.messages {
		if record.SessionID == snapshot.Session.ID {
			delete(s.messages, id)
		}
	}
	for _, record := range snapshot.Messages {
		s.messages[record.ID] = record
	}
	s.sessions[snapshot.Session.ID] = clonePlanRecoveryRecord(snapshot.Session)
	return nil
}
func (s *planRecoveryStore) SaveConfig(_ context.Context, config domainmodel.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configs == nil {
		s.configs = make(map[string]domainmodel.Config)
	}
	s.configs[config.ID] = config
	return nil
}
func (s *planRecoveryStore) GetConfig(_ context.Context, id string) (domainmodel.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config, ok := s.configs[id]
	if !ok {
		return domainmodel.Config{}, repository.ErrNotFound
	}
	return config, nil
}
func (s *planRecoveryStore) ListConfigs(context.Context) ([]domainmodel.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var configs []domainmodel.Config
	for _, config := range s.configs {
		configs = append(configs, config)
	}
	return configs, nil
}
func (s *planRecoveryStore) DeleteConfig(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.configs, id)
	return nil
}

func clonePlanRecoveryRecord(record repository.SessionRecord) repository.SessionRecord {
	record.CurrentPlan = agentplan.Clone(record.CurrentPlan)
	record.AllowedTools = append([]string(nil), record.AllowedTools...)
	record.PermissionCeiling = session.ClonePermissionCeiling(record.PermissionCeiling)
	record.RAGSettings = session.CloneRAGSettings(record.RAGSettings)
	record.GenerationSettings = generation.Clone(record.GenerationSettings)
	record.Usage, record.LastUsage = clonePlanRecoveryUsage(record.Usage), clonePlanRecoveryUsage(record.LastUsage)
	if record.CompactionCheckpoint != nil {
		record.CompactionCheckpoint = record.CompactionCheckpoint.Clone()
	}
	return record
}
func clonePlanRecoveryUsage(usage *repository.TokenUsageRecord) *repository.TokenUsageRecord {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
}
