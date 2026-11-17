package generation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	chatmessages "myai/core/adapter/persistence/chatmessage/repository"
	journaladapter "myai/core/adapter/persistence/outbox/journal"
	generationcommand "myai/core/application/chat/generation/command"
	runtimeservice "myai/core/application/runtime/service"
	modelport "myai/core/port/model"
	outbox "myai/core/port/outbox"
	repository "myai/core/port/repository"
	"myai/core/session"
)

type queueSink func(context.Context, outbox.Batch) error

func (f queueSink) Apply(ctx context.Context, b outbox.Batch) error { return f(ctx, b) }

type concurrentExecutor struct{}

func (concurrentExecutor) Submit(task func()) error { go task(); return nil }

type ackFailureJournal struct {
	outbox.Journal
	failure error
}

func (j ackFailureJournal) Ack(context.Context, int64) error { return j.failure }
func durableTestJournal(t *testing.T, path string, options journaladapter.Options) *journaladapter.Journal {
	t.Helper()
	options.Scope = outbox.Hash([]byte("queue-test"))
	j, err := journaladapter.Open(path, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}
func queueBatch(id, sessionID string) outbox.Batch {
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	return outbox.Batch{Version: outbox.Version, ID: id, SessionID: sessionID, Messages: []repository.MessageRecord{{ID: id, SessionID: sessionID, Role: repository.RoleUser, Content: "original " + id, CreatedAt: at, Sequence: at.UnixNano()}}}
}

func TestDurableQueueReopensBackendSuccessBeforeAckWindow(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private", "outbox.db")
	journal := durableTestJournal(t, path, journaladapter.Options{})
	failure := errors.New("ack unavailable")
	var attempts []repository.MessageRecord
	sink := queueSink(func(_ context.Context, batch outbox.Batch) error {
		attempts = append(attempts, batch.Messages...)
		return nil
	})
	queue, err := NewDurableSessionQueue(runtimeservice.AsyncTaskService{}, ackFailureJournal{journal, failure}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.SubmitRecords(ctx, queueBatch("message", "session")); err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatal("backend did not commit before failed ACK")
	}
	if err := queue.Flush(ctx); !errors.Is(err, failure) {
		t.Fatalf("ACK failure invisible: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	journal = durableTestJournal(t, path, journaladapter.Options{})
	fresh, err := NewDurableSessionQueue(runtimeservice.AsyncTaskService{}, journal, sink)
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 3 || !reflect.DeepEqual(attempts[0], attempts[2]) {
		t.Fatal("ACK window changed immutable records")
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	journal = durableTestJournal(t, path, journaladapter.Options{})
	done, err := NewDurableSessionQueue(runtimeservice.AsyncTaskService{}, journal, sink)
	if err != nil {
		t.Fatal(err)
	}
	if err := done.Flush(ctx); err != nil || len(attempts) != 3 {
		t.Fatal("done batch repeated after restart")
	}
}

func TestDurableQueueRejectsClosedJournalWithoutAcceptingTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "outbox.db")
	journal := durableTestJournal(t, path, journaladapter.Options{})
	called := 0
	queue, err := NewDurableSessionQueue(runtimeservice.AsyncTaskService{}, journal, queueSink(func(context.Context, outbox.Batch) error { called++; return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := queue.SubmitRecords(context.Background(), queueBatch("rejected", "session")); !errors.Is(err, outbox.ErrClosed) {
		t.Fatalf("closed journal accepted: %v", err)
	}
	if called != 0 || len(queue.queues) != 0 {
		t.Fatal("journal refusal scheduled backend task")
	}
	if err := queue.SubmitTask("session", func() error { return nil }); !errors.Is(err, outbox.ErrClosureNotDurable) {
		t.Fatal("durable queue accepted closure")
	}
}

func TestDurableQueueFailedHeadRejectsReplacementWithoutLateReplay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private", "outbox.db")
	journal := durableTestJournal(t, path, journaladapter.Options{})
	failure := errors.New("backend unavailable")
	queue, err := NewDurableSessionQueue(runtimeservice.AsyncTaskService{}, journal, queueSink(func(context.Context, outbox.Batch) error { return failure }))
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.SubmitRecords(ctx, queueBatch("pending", "session")); err != nil {
		t.Fatal(err)
	}
	replacements := 0
	if err := queue.SubmitAndWait(ctx, "session", func() error { replacements++; return nil }); !errors.Is(err, failure) {
		t.Fatalf("replacement did not receive failed-head result: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	journal = durableTestJournal(t, path, journaladapter.Options{})
	fresh, err := NewDurableSessionQueue(runtimeservice.AsyncTaskService{}, journal, queueSink(func(context.Context, outbox.Batch) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if replacements != 0 {
		t.Fatal("rolled back replacement executed after restart")
	}
}

func TestDurableQueueBlockPausedHeadWithoutExecutorAndKeepTombstone(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private", "outbox.db")
	journal := durableTestJournal(t, path, journaladapter.Options{})
	executor := &queuedExecutor{}
	calls := 0
	queue, err := NewDurableSessionQueue(runtimeservice.AsyncTaskService{Executor: executor}, journal, queueSink(func(context.Context, outbox.Batch) error { calls++; return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.SubmitRecords(ctx, queueBatch("old", "session")); err != nil {
		t.Fatal(err)
	}
	if err := queue.BlockSession(ctx, "session"); err != nil {
		t.Fatal(err)
	}
	executor.tasks[0]()
	if calls != 0 {
		t.Fatal("queued backend ran after tombstone")
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	journal = durableTestJournal(t, path, journaladapter.Options{})
	fresh, err := NewDurableSessionQueue(runtimeservice.AsyncTaskService{}, journal, queueSink(func(context.Context, outbox.Batch) error { calls++; return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.SubmitRecords(ctx, queueBatch("new", "session")); !errors.Is(err, outbox.ErrBlocked) {
		t.Fatalf("tombstone lost: %v", err)
	}
	if err := fresh.ResumeSession(ctx, "session"); err != nil {
		t.Fatal(err)
	}
	if err := fresh.SubmitRecords(ctx, queueBatch("new", "session")); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("resumed new append did not run once")
	}
}

func TestDurableQueueConcurrentRecoveryAcrossSessions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "private", "outbox.db")
	journal := durableTestJournal(t, path, journaladapter.Options{})
	const sessions = 24
	for index := 0; index < sessions; index++ {
		for step := 0; step < 3; step++ {
			if _, err := journal.Append(ctx, queueBatch(fmt.Sprintf("%d-%d", index, step), fmt.Sprint(index))); err != nil {
				t.Fatal(err)
			}
		}
	}
	var mu sync.Mutex
	seen := make(map[string][]string)
	queue, err := NewDurableSessionQueue(runtimeservice.AsyncTaskService{Executor: concurrentExecutor{}}, journal, queueSink(func(_ context.Context, batch outbox.Batch) error {
		mu.Lock()
		defer mu.Unlock()
		seen[batch.SessionID] = append(seen[batch.SessionID], batch.Messages[0].ID)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < sessions; index++ {
		want := []string{fmt.Sprintf("%d-0", index), fmt.Sprintf("%d-1", index), fmt.Sprintf("%d-2", index)}
		if !reflect.DeepEqual(seen[fmt.Sprint(index)], want) {
			t.Fatalf("session %d order=%v", index, seen[fmt.Sprint(index)])
		}
	}
}

func TestAcceptMessageAPIsReturnJournalRefusalAndSyncFallbackFailure(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private", "outbox.db")
	journal := durableTestJournal(t, path, journaladapter.Options{MaxPending: 1})
	executor := &queuedExecutor{}
	queue, err := NewDurableSessionQueue(runtimeservice.AsyncTaskService{Executor: executor}, journal, queueSink(func(context.Context, outbox.Batch) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	writer := chatmessages.Writer{}
	user := UserMessagePersistence{Messages: writer, Queue: queue}
	if err := user.AcceptUserMessage(ctx, generationcommand.PersistUserMessage{SessionID: "session", Input: "accepted"}); err != nil {
		t.Fatal(err)
	}
	assistant := Persistence{Messages: writer, Queue: queue}
	if err := assistant.AcceptAssistant(ctx, &session.Session{ID: "session"}, modelport.ChatResult{Content: "refused"}); !errors.Is(err, outbox.ErrCapacity) {
		t.Fatalf("capacity refusal=%v", err)
	}
	if len(executor.tasks) != 1 {
		t.Fatal("refused assistant scheduled additional task")
	}
	failure := errors.New("database failure")
	syncUser := UserMessagePersistence{Messages: failingAcceptWriter{failure}}
	if err := syncUser.AcceptUserMessage(ctx, generationcommand.PersistUserMessage{SessionID: "session"}); !errors.Is(err, failure) {
		t.Fatalf("sync fallback=%v", err)
	}
}

type failingAcceptWriter struct{ failure error }

func (w failingAcceptWriter) SaveUserMessage(context.Context, generationcommand.PersistUserMessage) error {
	return w.failure
}
