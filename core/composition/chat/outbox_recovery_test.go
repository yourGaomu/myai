package chat

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	generationadapter "myai/core/adapter/chat/generation"
	outboxjournal "myai/core/adapter/persistence/outbox/journal"
	outboxrepository "myai/core/adapter/persistence/outbox/repository"
	memorysession "myai/core/adapter/session/memory"
	runtimeservice "myai/core/application/runtime/service"
	modelport "myai/core/port/model"
	outbox "myai/core/port/outbox"
	persistenceport "myai/core/port/persistence"
	repository "myai/core/port/repository"
	chatservice "myai/core/service"
	"myai/core/session"
	"myai/core/tool"
)

var errOutboxRecoveryBackend = errors.New("test message backend unavailable")

func TestOutboxRecoveryCompositionReopensPendingRecordsWithoutRepeatingEffects(t *testing.T) {
	for _, partial := range []bool{false, true} {
		name := "before_first_save"
		if partial {
			name = "partial_tool_batch"
		}
		t.Run(name, func(t *testing.T) {
			ctx := planRecoveryContext(t)
			h := newOutboxRecoveryHarness(t, outboxjournal.Options{})
			h.sendToolTurn(t, ctx)
			live, err := h.memory.Snapshot(h.sessionID)
			if err != nil {
				t.Fatal(err)
			}
			accepted := outboxRecoveryPending(t, ctx, h.journal)
			want := outboxRecoveryMessages(accepted)
			assertOutboxRecoveryRoles(t, want)
			if len(h.store.messageRecords(h.sessionID)) != 0 || h.async.pending() == 0 {
				t.Fatal("generation did not leave backend writes delayed behind durable acceptance")
			}

			// Fail inside one real multi-record tool batch, after preceding entries
			// have already received ACKs. The same batch must survive partial save.
			failedEntry, saved := 0, 0
			if partial {
				failedEntry = -1
				for index, entry := range accepted {
					if len(entry.Batch.Messages) > 1 && outboxRecoveryHasRole(entry.Batch.Messages, repository.RoleTool) {
						failedEntry, saved = index, saved+1
						break
					}
					saved += len(entry.Batch.Messages)
				}
				if failedEntry < 0 {
					t.Fatal("composition did not durably accept a multi-record tool round")
				}
			}
			h.store.failAfter = saved
			h.async.drain()
			assertOutboxRecoveryRecords(t, want[:saved+1], h.store.attempts())
			assertOutboxRecoveryStored(t, h.store, h.sessionID, want[:saved])
			pending := outboxRecoveryPending(t, ctx, h.journal)
			if len(pending) != len(accepted)-failedEntry || pending[0].Sequence != accepted[failedEntry].Sequence {
				t.Fatalf("failed head was discarded or later work overtook it: pending=%d accepted=%d failed index=%d", len(pending), len(accepted), failedEntry)
			}
			stats := outboxRecoveryStats(t, ctx, h.journal)
			if stats.Done != failedEntry || stats.Pending != len(pending) {
				t.Fatalf("ACK preceded complete backend success: %+v", stats)
			}
			h.assertCalls(t, 2, 1)

			attempted := len(h.store.attempts())
			if err := h.journal.Close(); err != nil {
				t.Fatalf("close accepted on-disk journal: %v", err)
			}
			h.store.failAfter = -1
			// This constructs a fresh queue from the reopened SQLite file. None of
			// the original executor's closures or in-memory sessions are drained.
			fresh := h.reopen(t)
			if err := fresh.chat.FlushPersistence(ctx); err != nil {
				t.Fatalf("flush a fresh recovered queue: %v", err)
			}
			assertOutboxRecoveryRecords(t, outboxRecoveryMessages(pending), h.store.attempts()[attempted:])
			assertOutboxRecoveryStored(t, h.store, h.sessionID, want)
			stats = outboxRecoveryStats(t, ctx, fresh.journal)
			if stats.Pending != 0 || stats.Done != len(accepted) {
				t.Fatalf("recovery did not ACK every accepted batch: %+v", stats)
			}
			if err := fresh.chat.LoadSession(ctx, h.sessionID); err != nil {
				t.Fatalf("cold load through real composition: %v", err)
			}
			loaded, err := fresh.memory.Snapshot(h.sessionID)
			if err != nil {
				t.Fatal(err)
			}
			assertPlanRecoveryTranscript(t, live.Messages, loaded.Messages)
			fresh.assertCalls(t, 2, 1)

			// Done receipts survive another reopen, including an explicit retry of
			// each original immutable envelope. No model, tool, or backend replay.
			attempted = len(h.store.attempts())
			if err := fresh.journal.Close(); err != nil {
				t.Fatal(err)
			}
			done := fresh.reopen(t)
			for _, entry := range accepted {
				if err := done.queue.SubmitRecords(ctx, entry.Batch); err != nil {
					t.Fatalf("retry an ACKed envelope: %v", err)
				}
			}
			if err := done.chat.FlushPersistence(ctx); err != nil {
				t.Fatal(err)
			}
			if len(h.store.attempts()) != attempted || len(outboxRecoveryPending(t, ctx, done.journal)) != 0 {
				t.Fatal("done records were replayed after reopening")
			}
			done.assertCalls(t, 2, 1)
			t.Logf("%d SQLite-accepted batches / %d stable records survived close/reopen with %d writes before failure; done receipts prevented another replay", len(accepted), len(want), saved)
		})
	}
}

func TestOutboxRecoveryCompositionLifecycleCancelsPendingBeforeBackendMutation(t *testing.T) {
	for _, deleting := range []bool{false, true} {
		name := "clear"
		if deleting {
			name = "delete"
		}
		t.Run(name, func(t *testing.T) {
			ctx := planRecoveryContext(t)
			h := newOutboxRecoveryHarness(t, outboxjournal.Options{})
			h.sendToolTurn(t, ctx)
			accepted := outboxRecoveryPending(t, ctx, h.journal)
			oldRecords := outboxRecoveryMessages(accepted)
			assertOutboxRecoveryRoles(t, oldRecords)
			h.store.failAfter = 0
			h.async.drain()
			if len(h.store.attempts()) != 1 || len(outboxRecoveryPending(t, ctx, h.journal)) != len(accepted) {
				t.Fatal("lifecycle fixture did not retain a failed durable head")
			}
			h.store.requireFence = true
			if deleting {
				if err := h.chat.DeleteSession(ctx, h.sessionID); err != nil {
					t.Fatalf("delete through ChatService lifecycle hooks: %v", err)
				}
			} else if err := h.chat.ClearCurrent(ctx); err != nil {
				t.Fatalf("clear through ChatService lifecycle hooks: %v", err)
			}
			if h.store.fenceChecks != 1 {
				t.Fatalf("backend mutation did not observe a committed journal barrier: checks=%d", h.store.fenceChecks)
			}
			stats := outboxRecoveryStats(t, ctx, h.journal)
			if stats.Pending != 0 || stats.Cancelled != len(accepted) {
				t.Fatalf("destructive lifecycle left old work replayable: %+v", stats)
			}
			record, err := h.store.GetSession(ctx, h.sessionID)
			if err != nil || record.Deleted != deleting {
				t.Fatalf("lifecycle backend state: deleted=%v error=%v", record.Deleted, err)
			}
			attempted := len(h.store.attempts())
			if err := h.journal.Close(); err != nil {
				t.Fatal(err)
			}
			h.store.failAfter = -1
			fresh := h.reopen(t)
			if err := fresh.chat.FlushPersistence(ctx); err != nil {
				t.Fatalf("flush after lifecycle and journal reopen: %v", err)
			}
			if len(h.store.attempts()) != attempted || len(h.store.messageRecords(h.sessionID)) != 0 {
				t.Fatal("old pending transcript revived after lifecycle and restart")
			}
			blocked, err := fresh.journal.Blocked(ctx, h.sessionID)
			if err != nil || blocked != deleting {
				t.Fatalf("reopened lifecycle barrier: blocked=%v error=%v", blocked, err)
			}
			for _, entry := range accepted {
				if err := fresh.queue.SubmitRecords(ctx, entry.Batch); !errors.Is(err, outbox.ErrBlocked) {
					t.Fatalf("cancelled envelope was accepted again: %v", err)
				}
			}
			fresh.assertCalls(t, 2, 1)
			if !deleting {
				if err := fresh.chat.LoadSession(ctx, h.sessionID); err != nil {
					t.Fatal(err)
				}
				if _, err := fresh.chat.SendMessageStreamForSession(ctx, h.sessionID, "再给一句简短答复", modelport.ChatStreamHandler{}); err != nil {
					t.Fatalf("new turn after successful clear and restart: %v", err)
				}
				if err := fresh.chat.FlushPersistence(ctx); err != nil {
					t.Fatal(err)
				}
				newRecords := h.store.messageRecords(h.sessionID)
				if len(newRecords) == 0 {
					t.Fatal("successful clear left the session permanently blocked")
				}
				for _, current := range newRecords {
					for _, old := range oldRecords {
						if current.ID == old.ID {
							t.Fatalf("new clear epoch revived old message %s", old.ID)
						}
					}
				}
				fresh.assertCalls(t, 3, 1)
			}
			t.Logf("ChatService %s cancelled %d real journal batches before backend mutation; reopening did not revive old messages", name, len(accepted))
		})
	}
}

func TestOutboxRecoveryCompositionJournalRejectionStopsMainGenerationAndTools(t *testing.T) {
	for _, full := range []bool{false, true} {
		name := "closed_journal"
		if full {
			name = "capacity_exhausted"
		}
		t.Run(name, func(t *testing.T) {
			ctx := planRecoveryContext(t)
			h := newOutboxRecoveryHarness(t, outboxjournal.Options{MaxPending: 1})
			wantErr := outbox.ErrClosed
			if full {
				at := time.Now().UTC()
				_, err := h.journal.Append(ctx, outbox.Batch{
					Version: outbox.Version, ID: "capacity-blocker", SessionID: "capacity-blocker-session",
					Metadata: &outbox.Metadata{CreatedAt: at, UpdatedAt: at},
				})
				if err != nil {
					t.Fatalf("fill real journal pending capacity: %v", err)
				}
				wantErr = outbox.ErrCapacity
			} else if err := h.journal.Close(); err != nil {
				t.Fatal(err)
			}
			_, err := h.chat.SendMessageStreamForSession(ctx, h.sessionID, "请核对本轮答复", modelport.ChatStreamHandler{})
			if !errors.Is(err, wantErr) {
				t.Fatalf("durable rejection was not returned to ChatService caller: got=%v want=%v", err, wantErr)
			}
			h.assertCalls(t, 0, 0)
			if h.model.classifications() > 1 || len(h.store.attempts()) != 0 || h.async.pending() != 0 {
				t.Fatalf("rejected turn started work: classification=%d writes=%d executor tasks=%d", h.model.classifications(), len(h.store.attempts()), h.async.pending())
			}
			if full {
				pending := outboxRecoveryPending(t, ctx, h.journal)
				if len(pending) != 1 || pending[0].Batch.ID != "capacity-blocker" {
					t.Fatal("rejected user turn entered the full journal")
				}
			}
			if err := h.journal.Close(); err != nil {
				t.Fatal(err)
			}
			fresh := h.reopen(t)
			if err := fresh.chat.FlushPersistence(ctx); err != nil {
				t.Fatal(err)
			}
			if len(h.store.messageRecords(h.sessionID)) != 0 || len(h.store.attempts()) != 0 {
				t.Fatal("rejected user turn became replayable after restart")
			}
			fresh.assertCalls(t, 0, 0)
			t.Logf("real journal %s rejection reached caller before main model/tool work; rejected user records were absent after reopening", name)
		})
	}
}

// The filesystem journal, typed record sink, queue and composition are real.
// Only the model, harmless tool, backend store and scheduling executor are fake.
// Reopening always creates a fresh memory Store and queue; a nil executor runs
// recovery synchronously so no worker can outlive the test's journal handle.
type outboxRecoveryHarness struct {
	chat      *chatservice.ChatService
	memory    *memorysession.Store
	store     *outboxRecoveryStore
	async     *planRecoveryDelayedExecutor
	model     *outboxRecoveryModel
	effect    *planRecoveryEffectTool
	tools     *tool.RegisterTools
	queue     *generationadapter.SessionQueue
	journal   *outboxjournal.Journal
	path      string
	options   outboxjournal.Options
	sessionID string
}

func newOutboxRecoveryHarness(t *testing.T, options outboxjournal.Options) *outboxRecoveryHarness {
	t.Helper()
	options.Scope = outbox.Hash([]byte("composition-test"))
	h := &outboxRecoveryHarness{
		store: &outboxRecoveryStore{planRecoveryStore: &planRecoveryStore{
			sessions: make(map[string]repository.SessionRecord), messages: make(map[string]repository.MessageRecord),
		}, failAfter: -1},
		async: &planRecoveryDelayedExecutor{}, model: &outboxRecoveryModel{},
		effect: &planRecoveryEffectTool{calls: make(map[string]int)}, tools: tool.NewRegisterTools(),
		path: filepath.Join(t.TempDir(), "journal", "outbox.db"), options: options,
	}
	h.tools.Register(h.effect)
	h.open(t, runtimeservice.AsyncTaskService{Executor: h.async})
	ctx := planRecoveryContext(t)
	var err error
	h.sessionID, err = h.chat.CreateSession(ctx, "Outbox recovery regression")
	if err != nil {
		t.Fatalf("create composed test session: %v", err)
	}
	if err := h.chat.SetPermissionModeForSession(ctx, h.sessionID, string(session.PermissionModeFull)); err != nil {
		t.Fatalf("allow only registered in-memory test tool: %v", err)
	}
	return h
}

func (h *outboxRecoveryHarness) open(t *testing.T, async runtimeservice.AsyncTaskService) {
	t.Helper()
	journal, err := outboxjournal.Open(h.path, h.options)
	if err != nil {
		t.Fatalf("open real temporary SQLite outbox: %v", err)
	}
	t.Cleanup(func() {
		if err := journal.Close(); err != nil {
			t.Errorf("close temporary outbox: %v", err)
		}
	})
	h.journal = journal
	h.store.fenceJournal = journal
	h.queue, err = generationadapter.NewDurableSessionQueue(async, journal, outboxrepository.Writer{
		Messages: h.store, Assets: h.store, Metadata: h.store, Sessions: h.store,
	})
	if err != nil {
		t.Fatalf("build durable queue from real journal: %v", err)
	}
	h.memory = memorysession.NewStore(planRecoveryModelID)
	h.chat = NewService(Configuration{
		Sessions: h.memory, Store: h.store, Async: async.Executor, PersistenceQueue: h.queue,
		DefaultModel: planRecoveryModelID, Models: &planRecoveryRegistry{model: h.model}, Tools: h.tools,
		HistoryStoreFactory: planRecoveryNoHistory{t: t},
	})
}

func (h *outboxRecoveryHarness) reopen(t *testing.T) *outboxRecoveryHarness {
	t.Helper()
	fresh := &outboxRecoveryHarness{
		store: h.store, model: h.model, effect: h.effect, tools: h.tools,
		path: h.path, options: h.options, sessionID: h.sessionID,
	}
	fresh.open(t, runtimeservice.AsyncTaskService{})
	if _, err := fresh.memory.GetSession(h.sessionID); err == nil {
		t.Fatal("fresh recovery composition reused the original live session")
	}
	return fresh
}

func (h *outboxRecoveryHarness) sendToolTurn(t *testing.T, ctx context.Context) {
	t.Helper()
	response, err := h.chat.SendMessageStreamForSession(ctx, h.sessionID, "请核对本轮答复", modelport.ChatStreamHandler{})
	if err != nil {
		t.Fatalf("generate a real composed user/tool/assistant turn: %v", err)
	}
	if response.Result.Content != "outbox-recovery-answer" {
		t.Fatalf("tool round did not reach its final answer: %q", response.Result.Content)
	}
	h.assertCalls(t, 2, 1)
}

func (h *outboxRecoveryHarness) assertCalls(t *testing.T, samples, effects int) {
	t.Helper()
	if got := h.model.samples(); got != samples {
		t.Fatalf("main generation samples=%d want=%d", got, samples)
	}
	h.effect.mu.Lock()
	defer h.effect.mu.Unlock()
	if h.effect.calls["first"] != effects || len(h.effect.calls) > 1 {
		t.Fatalf("test tool effects=%v want first=%d", h.effect.calls, effects)
	}
}

type outboxRecoveryModel struct {
	mu                  sync.Mutex
	sampleCount         int
	classificationCount int
}

func (m *outboxRecoveryModel) Generate(_ context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(request.Messages) > 0 && strings.HasPrefix(request.Messages[0].Text(), "You are the intent classifier for a coding assistant.") {
		m.classificationCount++
		return modelport.ChatResult{Content: `{"intent":"conversation","confidence":1,"reason":"test reply"}`}, nil
	}
	m.sampleCount++
	if m.sampleCount == 1 {
		for _, candidate := range request.Tools {
			if candidate.Function != nil && candidate.Function.Name == planRecoveryToolName {
				return modelport.ChatResult{ToolCalls: []modelport.ToolCall{{
					ID: "effect-first", Type: "function", Name: planRecoveryToolName, Arguments: `{"step":"first"}`,
				}}}, nil
			}
		}
		return modelport.ChatResult{}, errors.New("composed model request omitted the harmless test tool")
	}
	if m.sampleCount == 2 {
		found := false
		for _, message := range request.Messages {
			if result, ok := message.FirstToolResult(); ok && result.ToolCallID == "effect-first" && result.Content == "effect:first" {
				found = true
			}
		}
		if !found {
			return modelport.ChatResult{}, errors.New("final sample did not receive the completed tool result")
		}
	}
	return modelport.ChatResult{Content: "outbox-recovery-answer"}, nil
}

func (m *outboxRecoveryModel) samples() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sampleCount
}

func (m *outboxRecoveryModel) classifications() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.classificationCount
}

// The existing fake enforces immutable IDs. This wrapper adds backend faults
// and observes journal fences without replacing any durability implementation.
type outboxRecoveryStore struct {
	*planRecoveryStore
	auditMu      sync.Mutex
	writes       []repository.MessageRecord
	failAfter    int
	saved        int
	requireFence bool
	fenceChecks  int
	fenceJournal *outboxjournal.Journal
}

var (
	_ persistenceport.Store                  = (*outboxRecoveryStore)(nil)
	_ repository.SessionMessageMetadataStore = (*outboxRecoveryStore)(nil)
	_ outbox.Sink                            = outboxrepository.Writer{}
	_ modelport.ChatModelPort                = (*outboxRecoveryModel)(nil)
)

func (s *outboxRecoveryStore) SaveMessage(ctx context.Context, record repository.MessageRecord) error {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	s.writes = append(s.writes, record)
	if s.failAfter >= 0 && s.saved >= s.failAfter {
		return errOutboxRecoveryBackend
	}
	if err := s.planRecoveryStore.SaveMessage(ctx, record); err != nil {
		return err
	}
	s.saved++
	return nil
}

func (s *outboxRecoveryStore) attempts() []repository.MessageRecord {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	return append([]repository.MessageRecord(nil), s.writes...)
}

func (s *outboxRecoveryStore) ClearMessages(ctx context.Context, id string) error {
	if err := s.observeFence(ctx, id); err != nil {
		return err
	}
	return s.planRecoveryStore.ClearMessages(ctx, id)
}

func (s *outboxRecoveryStore) MarkSessionDeleted(ctx context.Context, id string, at time.Time) error {
	if err := s.observeFence(ctx, id); err != nil {
		return err
	}
	return s.planRecoveryStore.MarkSessionDeleted(ctx, id, at)
}

func (s *outboxRecoveryStore) observeFence(ctx context.Context, id string) error {
	if !s.requireFence {
		return nil
	}
	blocked, err := s.fenceJournal.Blocked(ctx, id)
	if err != nil {
		return err
	}
	if !blocked {
		return errors.New("backend lifecycle mutation preceded the durable session fence")
	}
	pending, err := s.fenceJournal.Pending(ctx)
	if err != nil {
		return err
	}
	for _, entry := range pending {
		if entry.Batch.SessionID == id {
			return errors.New("backend lifecycle mutation preceded pending-record cancellation")
		}
	}
	s.fenceChecks++
	return nil
}

func outboxRecoveryPending(t *testing.T, ctx context.Context, journal *outboxjournal.Journal) []outbox.Entry {
	t.Helper()
	entries, err := journal.Pending(ctx)
	if err != nil {
		t.Fatalf("read real journal pending records: %v", err)
	}
	return entries
}

func outboxRecoveryStats(t *testing.T, ctx context.Context, journal *outboxjournal.Journal) outbox.Stats {
	t.Helper()
	stats, err := journal.Stats(ctx)
	if err != nil {
		t.Fatalf("read real journal receipts: %v", err)
	}
	return stats
}

func outboxRecoveryMessages(entries []outbox.Entry) []repository.MessageRecord {
	var messages []repository.MessageRecord
	for _, entry := range entries {
		messages = append(messages, entry.Batch.Messages...)
	}
	return messages
}

func outboxRecoveryHasRole(records []repository.MessageRecord, role string) bool {
	for _, record := range records {
		if record.Role == role {
			return true
		}
	}
	return false
}

func assertOutboxRecoveryRoles(t *testing.T, records []repository.MessageRecord) {
	t.Helper()
	for _, role := range []string{repository.RoleUser, repository.RoleAssistant, repository.RoleToolCall, repository.RoleTool} {
		if !outboxRecoveryHasRole(records, role) {
			t.Fatalf("composition did not durably accept role %s", role)
		}
	}
	identities := make(map[string]bool)
	var sequence int64
	for _, record := range records {
		if record.ID == "" || record.MessageID == "" || record.CreatedAt.IsZero() || record.Sequence <= 0 || record.Sequence < sequence || identities[record.ID] {
			t.Fatalf("accepted record has unstable identity, duplicate ID or wrong session order: %s/%d", record.ID, record.Sequence)
		}
		identities[record.ID], sequence = true, record.Sequence
	}
}

func assertOutboxRecoveryStored(t *testing.T, store *outboxRecoveryStore, sessionID string, want []repository.MessageRecord) {
	t.Helper()
	ordered := append([]repository.MessageRecord(nil), want...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Sequence != ordered[j].Sequence {
			return ordered[i].Sequence < ordered[j].Sequence
		}
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})
	assertOutboxRecoveryRecords(t, ordered, store.messageRecords(sessionID))
}

func assertOutboxRecoveryRecords(t *testing.T, want, got []repository.MessageRecord) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("backend record count=%d want=%d", len(got), len(want))
	}
	for index := range want {
		left, right := want[index], got[index]
		left.CreatedAt, right.CreatedAt = left.CreatedAt.UTC(), right.CreatedAt.UTC()
		if !reflect.DeepEqual(left, right) {
			t.Fatalf("backend replay changed record payload, identity or order at %d: got=%s/%d/%s want=%s/%d/%s", index, right.ID, right.Sequence, right.Role, left.ID, left.Sequence, left.Role)
		}
	}
}
