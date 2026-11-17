package repository

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	generationadapter "myai/core/adapter/chat/generation"
	generationcommand "myai/core/application/chat/generation/command"
	runtimeservice "myai/core/application/runtime/service"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	repository "myai/core/port/repository"
)

func TestRecorderSavesToolExecutionRecords(t *testing.T) {
	persistence := &fakePersistence{}
	recorder := Recorder{
		Persistence: persistence,
		IDs:         &sequentialIDs{},
		RunAsync: func(task func()) error {
			task()
			return nil
		},
	}

	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{
		Entries: []domaintool.ExecutionEntry{{
			Kind:       domaintool.ExecutionEntryToolCall,
			SessionID:  "session-1",
			ToolCallID: "call-1",
		}},
		Assets: []domaintool.SharedAsset{{SessionID: "session-1", ShortCode: "asset-code"}},
	})

	if len(persistence.messages) != 1 || persistence.messages[0].ID != "id-1" || persistence.messages[0].Role != repository.RoleToolCall {
		t.Fatalf("expected message record to be saved, got %#v", persistence.messages)
	}
	if len(persistence.assets) != 1 || persistence.assets[0].ID != "id-2" || persistence.assets[0].ShortCode != "asset-code" {
		t.Fatalf("expected asset record to be saved, got %#v", persistence.assets)
	}
}

func TestRecorderReportsPersistenceErrors(t *testing.T) {
	persistence := &fakePersistence{
		messageErr: errors.New("message failed"),
		assetErr:   errors.New("asset failed"),
	}
	var reported error
	recorder := Recorder{
		Persistence: persistence,
		IDs:         &sequentialIDs{},
		RunAsync: func(task func()) error {
			task()
			return nil
		},
		OnError: func(err error) {
			reported = err
		},
	}

	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{
		Entries: []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolResult}},
		Assets:  []domaintool.SharedAsset{{}},
	})

	if reported == nil {
		t.Fatal("expected persistence error to be reported")
	}
	if text := reported.Error(); !strings.Contains(text, "message failed") || !strings.Contains(text, "asset failed") {
		t.Fatalf("expected combined persistence error, got %q", text)
	}
}

func TestRecorderRetriesSamePreparedIdentitiesAfterAssetFailure(t *testing.T) {
	failure := errors.New("asset unavailable")
	persistence := newRetryPersistence()
	persistence.assetFailures = 1
	persistence.failure = failure
	ids := &sequentialIDs{}
	var queued func() error
	var reported []error
	recorder := Recorder{
		Persistence: persistence, IDs: ids,
		RunRetryForSession: func(sessionID string, task func() error) error {
			if sessionID != "session-1" {
				t.Fatalf("queued session=%q", sessionID)
			}
			queued = task
			return nil
		},
		RunAsyncForSession: func(string, func()) error { t.Fatal("used legacy runner"); return nil },
		RunAsync:           func(func()) error { t.Fatal("used unkeyed runner"); return nil },
		OnError:            func(err error) { reported = append(reported, err) },
	}
	expiresAt := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	command := generationcommand.ToolExecutionRecord{
		SessionID: "session-1",
		Entries: []domaintool.ExecutionEntry{{
			Kind: domaintool.ExecutionEntryToolResult, ToolCallID: "call-1", ToolName: "share_file", Content: "original",
		}},
		Assets: []domaintool.SharedAsset{{ToolCallID: "call-1", ToolName: "share_file", ShortCode: "original", ExpiresAt: &expiresAt}},
	}
	recorder.RecordToolExecution(context.Background(), command)
	if queued == nil || ids.next != 2 || len(persistence.messageAttempts) != 0 || len(persistence.assetAttempts) != 0 {
		t.Fatalf("identities not prepared before queue: task=%v ids=%d", queued != nil, ids.next)
	}
	command.Entries[0].Content = "changed after enqueue"
	command.Assets[0].ShortCode = "changed after enqueue"
	expiresAt = expiresAt.Add(time.Hour)
	if err := queued(); !errors.Is(err, failure) {
		t.Fatalf("save failure swallowed by queue task: %v", err)
	}
	if len(persistence.messages) != 1 || len(persistence.assets) != 0 || len(reported) != 1 || !errors.Is(reported[0], failure) {
		t.Fatalf("unexpected partial commit: messages=%d assets=%d errors=%v", len(persistence.messages), len(persistence.assets), reported)
	}
	if err := queued(); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if !reflect.DeepEqual(persistence.messageAttempts[0], persistence.messageAttempts[1]) || !reflect.DeepEqual(persistence.assetAttempts[0], persistence.assetAttempts[1]) {
		t.Fatal("retry regenerated record identities or payloads")
	}
	message, asset := persistence.messageAttempts[0], persistence.assetAttempts[0]
	if message.Content != "original" || asset.ShortCode != "original" || asset.ExpiresAt.Equal(expiresAt) {
		t.Fatal("queued records retained caller mutations")
	}
	if message.ID != "id-1" || asset.ID != "id-2" || message.SessionID != asset.SessionID || message.SessionID != "session-1" || message.ToolCallID != asset.ToolCallID {
		t.Fatalf("message/asset identity diverged: message=%#v asset=%#v", message, asset)
	}
	if message.CreatedAt.IsZero() || asset.CreatedAt.IsZero() || message.Sequence != message.CreatedAt.UnixNano() {
		t.Fatalf("fallback metadata was not frozen: message=%#v asset=%#v", message, asset)
	}
	if len(persistence.messages) != 1 || len(persistence.assets) != 1 || ids.next != 2 || len(reported) != 1 {
		t.Fatal("retry duplicated successfully committed records or success reporting")
	}
}

func TestRecorderRetriesSnapshotMessagesWithoutRemapping(t *testing.T) {
	failure := errors.New("message unavailable")
	persistence := newRetryPersistence()
	persistence.messageFailures, persistence.failure = 1, failure
	ids := &sequentialIDs{}
	var queued func() error
	recorder := Recorder{
		Persistence: persistence, IDs: ids,
		RunRetryForSession: func(_ string, task func() error) error { queued = task; return nil },
	}
	message := domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "call-1", Name: "share_file", Content: "original result"})
	command := generationcommand.ToolExecutionRecord{
		SessionID: "session-1", Messages: []domainmessage.Message{message},
		Entries: []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolResult, Content: "must not be written twice"}},
		Assets:  []domaintool.SharedAsset{{ID: "live-asset", SessionID: "session-1", ToolCallID: "call-1", ToolName: "share_file"}},
	}
	recorder.RecordToolExecution(context.Background(), command)
	if ids.next != 1 {
		t.Fatalf("snapshot mapping did not happen before queue: IDs=%d", ids.next)
	}
	command.Messages[0].Parts[0].ToolResult.Content = "caller changed result"
	if err := queued(); !errors.Is(err, failure) {
		t.Fatalf("message failure swallowed: %v", err)
	}
	if len(persistence.assets) != 1 {
		t.Fatal("message failure prevented independent asset persistence")
	}
	if err := queued(); err != nil {
		t.Fatal(err)
	}
	if len(persistence.messageAttempts) != 2 || len(persistence.assetAttempts) != 2 || ids.next != 1 {
		t.Fatal("snapshot or entry records were remapped on retry")
	}
	if !reflect.DeepEqual(persistence.messageAttempts[0], persistence.messageAttempts[1]) || !reflect.DeepEqual(persistence.assetAttempts[0], persistence.assetAttempts[1]) {
		t.Fatal("snapshot fallback timestamps/identities changed on retry")
	}
	got := persistence.messageAttempts[1]
	if got.Content != "original result" || got.ID != "id-1" || got.ToolCallID != "call-1" || got.CreatedAt.IsZero() {
		t.Fatalf("unexpected snapshot record: %#v", got)
	}
	if len(persistence.messages) != 1 || len(persistence.assets) != 1 {
		t.Fatal("partial success duplicated on snapshot retry")
	}
}

func TestRecorderPreservesSessionMessageMetadata(t *testing.T) {
	persistence := newRetryPersistence()
	createdAt := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	message := domainmessage.Assistant("assistant text", "raw reasoning", []domainmessage.ToolCall{{ID: "call-1", Name: "share_file"}})
	message.ID, message.RecordIDs, message.Sequence, message.CreatedAt = "live-message", []string{"live-text", "live-call"}, 42, createdAt
	recorder := Recorder{Persistence: persistence, IDs: &sequentialIDs{}}
	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{
		SessionID: "session-1", Messages: []domainmessage.Message{message},
		Entries: []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolCall, ToolCallID: "call-1"}},
		Assets:  []domaintool.SharedAsset{{ID: "live-asset", ToolCallID: "call-1", CreatedAt: createdAt}},
	})
	if len(persistence.messageAttempts) != 2 || len(persistence.assetAttempts) != 1 {
		t.Fatal("actual session messages were duplicated by entry fallback")
	}
	for index, record := range persistence.messageAttempts {
		if record.ID != message.RecordIDs[index] || record.MessageID != message.ID || record.Sequence != message.Sequence || record.CreatedAt != createdAt {
			t.Fatalf("live cursor metadata changed: %#v", record)
		}
	}
	if persistence.messageAttempts[0].Reasoning != message.Reasoning || persistence.assetAttempts[0].ID != "live-asset" || persistence.assetAttempts[0].CreatedAt != createdAt {
		t.Fatal("live assistant/asset metadata changed")
	}
}

func TestRecorderSessionQueueRetainsFailedBatchAndDoesNotRepeatSuccess(t *testing.T) {
	failure := errors.New("asset offline")
	persistence := newRetryPersistence()
	persistence.assetFailures, persistence.failure = 2, failure
	executor := &recorderExecutor{}
	queue := generationadapter.NewSessionQueue(runtimeservice.AsyncTaskService{Executor: executor})
	ids := &sequentialIDs{}
	recorder := Recorder{Persistence: persistence, IDs: ids, RunRetryForSession: queue.SubmitTask}
	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{
		SessionID: "session-1",
		Entries:   []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolResult, ToolCallID: "call-1"}},
		Assets:    []domaintool.SharedAsset{{ToolCallID: "call-1"}},
	})
	var afterBatch int
	if err := queue.SubmitTask("session-1", func() error { afterBatch++; return nil }); err != nil {
		t.Fatal(err)
	}
	executor.drain()
	if afterBatch != 0 || len(persistence.assets) != 0 {
		t.Fatal("session queue treated failed tool persistence as success")
	}
	executor.inline = true
	if err := queue.Flush(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("pending failure lost: %v", err)
	}
	if afterBatch != 0 {
		t.Fatal("later task bypassed failed tool batch")
	}
	if err := queue.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if afterBatch != 1 || len(persistence.messages) != 1 || len(persistence.assets) != 1 || ids.next != 2 {
		t.Fatal("tool retry lost identities, duplicated records or failed to release queue")
	}
	for index := range persistence.messageAttempts {
		if !reflect.DeepEqual(persistence.messageAttempts[0], persistence.messageAttempts[index]) || !reflect.DeepEqual(persistence.assetAttempts[0], persistence.assetAttempts[index]) {
			t.Fatal("real queue retry changed records")
		}
	}
	attempts := len(persistence.messageAttempts)
	if err := queue.Flush(context.Background()); err != nil || len(persistence.messageAttempts) != attempts || afterBatch != 1 {
		t.Fatal("successful batch executed again on flush")
	}
}

func TestRecorderAcceptedWorkSurvivesCallerCancellationAndReleasesAttemptContext(t *testing.T) {
	for _, cancelBefore := range []bool{false, true} {
		t.Run(fmt.Sprint("cancelBefore=", cancelBefore), func(t *testing.T) {
			persistence := newRetryPersistence()
			var queued func() error
			recorder := Recorder{
				Persistence: persistence, Timeout: time.Second,
				RunRetryForSession: func(_ string, task func() error) error { queued = task; return nil },
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelBefore {
				cancel()
			}
			recorder.RecordToolExecution(ctx, generationcommand.ToolExecutionRecord{
				SessionID: "session-1", Entries: []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolResult}},
			})
			cancel()
			if queued == nil {
				t.Fatal("caller cancellation discarded accepted records")
			}
			if err := queued(); err != nil {
				t.Fatalf("caller cancellation reached durable save: %v", err)
			}
			if len(persistence.contexts) != 1 || persistence.contextErrors[0] != nil {
				t.Fatal("save context was canceled before saving")
			}
			saveCtx := persistence.contexts[0]
			deadline, ok := saveCtx.Deadline()
			if !ok || time.Until(deadline) > recorder.Timeout || !errors.Is(saveCtx.Err(), context.Canceled) {
				t.Fatal("attempt context was not bounded and released")
			}
		})
	}
}

func TestRecorderTimeoutFailureReturnsToQueueAndRetryGetsFreshContext(t *testing.T) {
	persistence := newRetryPersistence()
	var queued func() error
	recorder := Recorder{
		Persistence: persistence, Timeout: -1,
		RunRetryForSession: func(_ string, task func() error) error { queued = task; return nil },
	}
	persistence.beforeSave = func(ctx context.Context) error {
		if len(persistence.contexts) == 1 {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	recorder.Timeout = time.Millisecond
	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{
		SessionID: "session-1", Entries: []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolResult}},
	})
	if err := queued(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout swallowed: %v", err)
	}
	if err := queued(); err != nil {
		t.Fatalf("retry reused expired context: %v", err)
	}
	if len(persistence.contexts) != 2 || persistence.contexts[0] == persistence.contexts[1] || persistence.contextErrors[1] != nil {
		t.Fatal("retry context was not refreshed")
	}
}

func TestRecorderRetryTaskReturnsAllErrors(t *testing.T) {
	messageFailure, assetFailure := errors.New("message failed"), errors.New("asset failed")
	persistence := &fakePersistence{messageErr: messageFailure, assetErr: assetFailure}
	var task func() error
	var reports []error
	recorder := Recorder{
		Persistence:        persistence,
		RunRetryForSession: func(_ string, save func() error) error { task = save; return nil },
		OnError:            func(err error) { reports = append(reports, err) },
	}
	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{
		SessionID: "session-1", Entries: []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolResult}}, Assets: []domaintool.SharedAsset{{}},
	})
	err := task()
	if !errors.Is(err, messageFailure) || !errors.Is(err, assetFailure) || len(reports) != 1 || !errors.Is(reports[0], messageFailure) || !errors.Is(reports[0], assetFailure) {
		t.Fatalf("errors lost or reported twice: task=%v reports=%v", err, reports)
	}
}

func TestRecorderReportsRetrySchedulingFailure(t *testing.T) {
	failure := errors.New("queue rejected")
	persistence := &fakePersistence{}
	var reported []error
	recorder := Recorder{
		Persistence:        persistence,
		RunRetryForSession: func(string, func() error) error { return failure },
		OnError:            func(err error) { reported = append(reported, err) },
	}
	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{
		SessionID: "session-1", Entries: []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolCall}},
	})
	if len(reported) != 1 || !errors.Is(reported[0], failure) || len(persistence.messages) != 0 {
		t.Fatalf("scheduling failure lost or unexpectedly saved: %v", reported)
	}
}

func TestRecorderKeepsLegacySessionRunnerAndSessionInference(t *testing.T) {
	for _, source := range []string{"entry", "asset", "empty"} {
		t.Run(source, func(t *testing.T) {
			persistence := &fakePersistence{}
			var queued func()
			want := "session-1"
			command := generationcommand.ToolExecutionRecord{}
			switch source {
			case "entry":
				command.Entries = []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolCall, SessionID: want}}
			case "asset":
				command.Assets = []domaintool.SharedAsset{{SessionID: want}}
			default:
				want = ""
				command.Entries = []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolResult}}
			}
			recorder := Recorder{
				Persistence: persistence,
				RunAsyncForSession: func(sessionID string, task func()) error {
					if sessionID != want {
						t.Fatalf("inferred session=%q want=%q", sessionID, want)
					}
					queued = task
					return nil
				},
				RunAsync: func(task func()) error { queued = task; return nil },
			}
			recorder.RecordToolExecution(context.Background(), command)
			if queued == nil || len(persistence.messages) != 0 || len(persistence.assets) != 0 {
				t.Fatal("legacy runner not used asynchronously")
			}
			queued()
			if len(persistence.messages)+len(persistence.assets) != 1 {
				t.Fatal("legacy runner did not save once")
			}
		})
	}
}

func TestRecorderRetryRunnerReceivesUnkeyedWorkAndSkipsEmptyCommands(t *testing.T) {
	calls := 0
	recorder := Recorder{
		Persistence: &fakePersistence{},
		RunRetryForSession: func(sessionID string, task func() error) error {
			calls++
			if sessionID != "" {
				t.Fatal("unexpected session")
			}
			return task()
		},
	}
	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{})
	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{Assets: []domaintool.SharedAsset{{}}})
	if calls != 1 {
		t.Fatalf("empty command enqueued or unkeyed task bypassed retry runner: %d", calls)
	}
}

type retryPersistence struct {
	messageAttempts []repository.MessageRecord
	assetAttempts   []repository.AssetRecord
	messages        map[string]repository.MessageRecord
	assets          map[string]repository.AssetRecord
	messageFailures int
	assetFailures   int
	failure         error
	contexts        []context.Context
	contextErrors   []error
	beforeSave      func(context.Context) error
}

func newRetryPersistence() *retryPersistence {
	return &retryPersistence{messages: make(map[string]repository.MessageRecord), assets: make(map[string]repository.AssetRecord)}
}

func (p *retryPersistence) observe(ctx context.Context) error {
	p.contexts = append(p.contexts, ctx)
	p.contextErrors = append(p.contextErrors, ctx.Err())
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.beforeSave != nil {
		return p.beforeSave(ctx)
	}
	return nil
}

func (p *retryPersistence) SaveMessage(ctx context.Context, record repository.MessageRecord) error {
	p.messageAttempts = append(p.messageAttempts, record)
	if err := p.observe(ctx); err != nil {
		return err
	}
	if p.messageFailures > 0 {
		p.messageFailures--
		return p.failure
	}
	if previous, exists := p.messages[record.ID]; exists && !reflect.DeepEqual(previous, record) {
		return errors.New("message identity reused for a different record")
	}
	p.messages[record.ID] = record
	return nil
}

func (p *retryPersistence) SaveAsset(ctx context.Context, record repository.AssetRecord) error {
	p.assetAttempts = append(p.assetAttempts, record)
	if err := p.observe(ctx); err != nil {
		return err
	}
	if p.assetFailures > 0 {
		p.assetFailures--
		return p.failure
	}
	if previous, exists := p.assets[record.ID]; exists && !reflect.DeepEqual(previous, record) {
		return errors.New("asset identity reused for a different record")
	}
	p.assets[record.ID] = record
	return nil
}

type recorderExecutor struct {
	tasks  []func()
	inline bool
}

func (e *recorderExecutor) Submit(task func()) error {
	if e.inline {
		task()
	} else {
		e.tasks = append(e.tasks, task)
	}
	return nil
}

func (e *recorderExecutor) drain() {
	for len(e.tasks) > 0 {
		task := e.tasks[0]
		e.tasks = e.tasks[1:]
		task()
	}
}

type sequentialIDs struct {
	next int
}

func (g *sequentialIDs) NewID() string {
	g.next++
	return fmt.Sprintf("id-%d", g.next)
}

type fakePersistence struct {
	messages   []repository.MessageRecord
	assets     []repository.AssetRecord
	messageErr error
	assetErr   error
}

func (p *fakePersistence) SaveMessage(ctx context.Context, record repository.MessageRecord) error {
	p.messages = append(p.messages, record)
	return p.messageErr
}

func (p *fakePersistence) SaveAsset(ctx context.Context, record repository.AssetRecord) error {
	p.assets = append(p.assets, record)
	return p.assetErr
}
