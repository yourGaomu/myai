package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	memoryadapter "myai/core/adapter/session/memory"
	contextservice "myai/core/application/chat/context/service"
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	generationservice "myai/core/application/chat/generation/service"
	sessioncommand "myai/core/application/session/command"
	currentservice "myai/core/application/session/current/service"
	lifecycleservice "myai/core/application/session/lifecycle/service"
	loadservice "myai/core/application/session/load/service"
	messageservice "myai/core/application/session/message/service"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	repository "myai/core/port/repository"
)

type callerAcceptanceSink struct {
	err      error
	commands []generationcommand.PersistUserMessage
	legacy   int
}

func (p *callerAcceptanceSink) PersistUserMessage(generationcommand.PersistUserMessage) { p.legacy++ }
func (p *callerAcceptanceSink) AcceptUserMessage(_ context.Context, command generationcommand.PersistUserMessage) error {
	p.commands = append(p.commands, command)
	return p.err
}

type callerAcceptanceModel struct{ calls int }

func (m *callerAcceptanceModel) Generate(context.Context, modelport.GenerateRequest) (modelport.ChatResult, error) {
	m.calls++
	if m.calls == 1 {
		return modelport.ChatResult{ToolCalls: []domainmessage.ToolCall{{ID: "read", Name: "read_file", Arguments: `{}`}}}, nil
	}
	return modelport.ChatResult{Content: "done"}, nil
}

type callerAcceptanceExecutor struct{ calls int }

func (e *callerAcceptanceExecutor) Execute(_ context.Context, command generationcommand.ToolExecution) (generationresult.ToolExecution, error) {
	e.calls++
	return generationresult.ToolExecution{Calls: command.Calls, Messages: []domainmessage.Message{domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "read", Name: "read_file", Content: "read evidence"})}}, nil
}

type callerAcceptanceIDs struct{}

func (callerAcceptanceIDs) NewRequestID() string { return "request" }

func TestCallerDurableRejectionGatesNewSourceIDsAndReleasesPendingBatch(t *testing.T) {
	memory := memoryadapter.NewStore("model")
	if err := memory.PutSession("session", "model", nil); err != nil {
		t.Fatal(err)
	}
	loader := loadservice.LoadService{Memory: memory}
	model, executor := &callerAcceptanceModel{}, &callerAcceptanceExecutor{}
	wantErr := errors.New("user journal rejected")
	sink := &callerAcceptanceSink{err: wantErr}
	chat := NewChatService(ChatDependencies{
		Models: semanticModelRegistry{model: model}, UserMessages: sink,
		MessageCommands: messageservice.CommandService{Memory: memory, Loader: loader},
		GenerationTasks: generationservice.TaskService{RequestIDs: callerAcceptanceIDs{}, Generator: generationservice.AssistantGenerationService{
			Models: semanticModelRegistry{model: model}, Contexts: contextservice.SnapshotService{},
			AgentRunner:       generationservice.AgentLoopService{Sessions: memory, Contexts: contextservice.SnapshotService{}, ToolExecutor: executor},
			ResponseCommitter: generationservice.ResponseCommitService{Memory: memory},
		}},
	})
	for _, sourceID := range []string{"first", "second", "third", "fourth"} {
		_, err := chat.ContinueSessionStreamForMessage(context.Background(), "session", "payload "+sourceID, domainmessage.SyntheticReasonSubagentResult, sourceID, "task_result", modelport.ChatStreamHandler{})
		if !errors.Is(err, wantErr) {
			t.Fatalf("source %s bypassed rejected acceptance: %v", sourceID, err)
		}
	}
	stored, _ := memory.Snapshot("session")
	pending := chat.pendingUserAcceptance["session"]
	if len(stored.Messages) != 2 || stored.Messages[1].SourceID != "first" || len(chat.pendingUserAcceptance) != 1 || len(pending.AppendedMessages) != 1 || pending.AppendedMessages[0].SourceID != "first" {
		t.Fatalf("rejected requests grew or replaced the pending batch: history=%+v pending=%+v", stored.Messages, pending)
	}
	if model.calls != 0 || executor.calls != 0 || sink.legacy != 0 || len(sink.commands) != 4 {
		t.Fatalf("rejection ran generation, a tool, or void fallback: model=%d tools=%d legacy=%d accept=%d", model.calls, executor.calls, sink.legacy, len(sink.commands))
	}
	originalID := sink.commands[0].AppendedMessages[0].ID
	for _, command := range sink.commands {
		if command.AppendedMessages[0].ID != originalID {
			t.Fatal("retry changed the original durable message identity")
		}
	}
	sink.err = nil
	response, err := chat.ContinueSessionStreamForMessage(context.Background(), "session", "payload second", domainmessage.SyntheticReasonSubagentResult, "second", "task_result", modelport.ChatStreamHandler{})
	if err != nil || response.Result.Content != "done" || len(chat.pendingUserAcceptance) != 0 || model.calls != 2 || executor.calls != 1 {
		t.Fatalf("successful retry did not release gate: response=%+v error=%v pending=%d model=%d tool=%d", response, err, len(chat.pendingUserAcceptance), model.calls, executor.calls)
	}
	if len(sink.commands) != 6 || sink.commands[4].AppendedMessages[0].ID != originalID || sink.commands[5].AppendedMessages[0].SourceID != "second" {
		t.Fatal("new input was not accepted after the original rejected batch")
	}
}

type callerLifecycleStore struct {
	mu         sync.Mutex
	record     repository.SessionRecord
	log        []string
	saveErr    error
	restoreErr error
	writes     int
	restores   int
}

func (s *callerLifecycleStore) note(event string) {
	s.mu.Lock()
	s.log = append(s.log, event)
	s.mu.Unlock()
}
func (s *callerLifecycleStore) events() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.log...)
}
func (s *callerLifecycleStore) GetSession(context.Context, string) (repository.SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.record, nil
}
func (s *callerLifecycleStore) RenameSession(_ context.Context, sessionID, title string) (repository.SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record.ID != sessionID || s.record.Deleted {
		return repository.SessionRecord{}, repository.ErrNotFound
	}
	s.record.ManualTitle = title
	s.record.TitleRevision++
	s.log = append(s.log, "rename")
	return s.record, nil
}

func (s *callerLifecycleStore) MarkSessionDeleted(context.Context, string, time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log, s.record.Deleted = append(s.log, "delete"), true
	s.writes++
	return nil
}
func (s *callerLifecycleStore) MarkSessionRestored(context.Context, string, time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = append(s.log, "restore")
	s.restores++
	if s.restoreErr != nil {
		return s.restoreErr
	}
	s.record.Deleted = false
	s.writes++
	return nil
}
func (s *callerLifecycleStore) ClearMessages(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = append(s.log, "clear")
	s.writes++
	return nil
}
func (s *callerLifecycleStore) ListMessages(context.Context, string) ([]repository.MessageRecord, error) {
	return nil, nil
}
func (s *callerLifecycleStore) Save(context.Context, sessioncommand.SaveSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = append(s.log, "save")
	s.writes++
	return s.saveErr
}
func (s *callerLifecycleStore) ContinuationAllowed(context.Context, string) (bool, error) {
	return true, nil
}
func (s *callerLifecycleStore) SetContinuationPaused(_ context.Context, _ string, paused bool) error {
	if paused {
		s.mu.Lock()
		s.log = append(s.log, "pause")
		s.writes++
		s.mu.Unlock()
	}
	return nil
}

func callerLifecycleChat(t *testing.T, memory *memoryadapter.Store, store *callerLifecycleStore) *ChatService {
	t.Helper()
	loader := loadservice.LoadService{Memory: memory, Sessions: store, Messages: store}
	return NewChatService(ChatDependencies{
		Models: autoPlanModelRegistry{}, CurrentState: currentservice.StateQueryService{Memory: memory},
		SessionLoader: loader, MessageCommands: messageservice.CommandService{Memory: memory, Loader: loader},
		SessionLifecycle:    lifecycleservice.UseCase{Lifecycle: lifecycleservice.LifecycleService{Memory: memory, Loader: loader, Sessions: store, Messages: store}, Persistence: store},
		ContinuationControl: store,
		BlockSession:        func(context.Context, string) error { store.note("block"); return nil },
		ResumeSession:       func(context.Context, string) error { store.note("resume"); return nil },
	})
}

func callerLifecycleMemory(t *testing.T) *memoryadapter.Store {
	t.Helper()
	memory := memoryadapter.NewStore("model")
	for _, id := range []string{"session", "other"} {
		if err := memory.PutSession(id, "model", []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "keep "+id)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := memory.UseSession("session"); err != nil {
		t.Fatal(err)
	}
	return memory
}

func callerReceiveError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not finish")
		return nil
	}
}

func TestCallerLifecycleWaitsForCancelledGenerationBeforeBlockAndStore(t *testing.T) {
	for _, operation := range []string{"clear", "delete"} {
		for _, blockFails := range []bool{false, true} {
			name := operation
			if blockFails {
				name += "_block_rejected"
			}
			t.Run(name, func(t *testing.T) {
				memory := callerLifecycleMemory(t)
				if operation == "delete" {
					_ = memory.UseSession("other")
				}
				store := &callerLifecycleStore{record: repository.SessionRecord{ID: "session", Model: "model"}}
				chat := callerLifecycleChat(t, memory, store)
				started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
				chat.dependencies.GenerationTasks = continuationGenerator(func(ctx context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
					close(started)
					<-ctx.Done()
					close(cancelled)
					<-release
					_, _, err := memory.AppendMessagesTo(command.Session.ID, domainmessage.Text(domainmessage.RoleAssistant, "late evidence"))
					if err != nil {
						return generationresult.GenerationResponse{}, err
					}
					store.note("generation-finished")
					return generationresult.GenerationResponse{}, ctx.Err()
				})
				wantErr := errors.New("tombstone rejected")
				blocked := make(chan struct{}, 1)
				chat.dependencies.BlockSession = func(context.Context, string) error {
					store.note("block")
					blocked <- struct{}{}
					if blockFails {
						return wantErr
					}
					return nil
				}
				generationDone := make(chan error, 1)
				go func() {
					_, err := chat.SendMessageStreamForSession(context.Background(), "session", "start", modelport.ChatStreamHandler{})
					generationDone <- err
				}()
				select {
				case <-started:
				case <-time.After(3 * time.Second):
					t.Fatal("generation did not start")
				}
				lifecycleDone := make(chan error, 1)
				go func() {
					if operation == "delete" {
						lifecycleDone <- chat.DeleteSession(context.Background(), "session")
					} else {
						lifecycleDone <- chat.ClearCurrent(context.Background())
					}
				}()
				select {
				case <-cancelled:
				case <-time.After(3 * time.Second):
					t.Fatal("lifecycle did not cancel generation")
				}
				select {
				case <-blocked:
					t.Fatal("block ran before generation finished")
				default:
				}
				releaseOnce.Do(func() { close(release) })
				if err := callerReceiveError(t, generationDone); !errors.Is(err, context.Canceled) {
					t.Fatalf("generation error=%v", err)
				}
				err := callerReceiveError(t, lifecycleDone)
				if blockFails {
					if !errors.Is(err, wantErr) || store.writes != 0 {
						t.Fatalf("block rejection modified store: error=%v writes=%d log=%v", err, store.writes, store.events())
					}
					if got := store.events(); !reflect.DeepEqual(got, []string{"generation-finished", "block"}) {
						t.Fatalf("wrong rejected order: %v", got)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				want := []string{"generation-finished", "block", "pause", "delete"}
				if operation == "clear" {
					want = []string{"generation-finished", "block", "pause", "clear", "save", "resume"}
				}
				if got := store.events(); !reflect.DeepEqual(got, want) {
					t.Fatalf("wrong lifecycle order: got=%v want=%v", got, want)
				}
				if operation == "clear" {
					snapshot, _ := memory.Snapshot("session")
					for _, message := range snapshot.Messages {
						if message.Text() == "late evidence" {
							t.Fatal("late generation survived clear")
						}
					}
				} else if _, err := memory.Snapshot("session"); err == nil {
					t.Fatal("deleted memory session remains")
				}
			})
		}
	}
}

func TestRenameSessionDoesNotWaitForOrCancelGeneration(t *testing.T) {
	memory := callerLifecycleMemory(t)
	store := &callerLifecycleStore{record: repository.SessionRecord{ID: "other", Title: "New chat"}}
	chat := callerLifecycleChat(t, memory, store)
	generationCtx, finish, err := chat.beginSessionGeneration(context.Background(), "other")
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := chat.RenameSession(ctx, "other", " Renamed ")
	if err != nil || result.SessionID != "other" || result.Title != "Renamed" || result.TitleRevision != 1 {
		t.Fatalf("rename result=%+v err=%v", result, err)
	}
	if generationCtx.Err() != nil || chat.CurrentSessionID() != "session" {
		t.Fatal("rename changed the running generation or current session")
	}
	if got := store.events(); !reflect.DeepEqual(got, []string{"rename"}) {
		t.Fatalf("rename touched other session state: %v", got)
	}
}

func TestCallerFailedClearRemainsBlockedAcrossRestartAndRetriesCapturedSession(t *testing.T) {
	memory := callerLifecycleMemory(t)
	wantErr := errors.New("save cleared session failed")
	store := &callerLifecycleStore{record: repository.SessionRecord{ID: "session", Model: "model"}, saveErr: wantErr}
	chat := callerLifecycleChat(t, memory, store)
	chat.pendingUserAcceptance = map[string]generationcommand.PersistUserMessage{"session": {SessionID: "session", Input: "discard rejected input"}}
	if err := chat.ClearCurrent(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("clear error=%v", err)
	}
	if len(chat.pendingUserAcceptance) != 0 {
		t.Fatal("clear retained rejected user cache")
	}
	if _, err := chat.SendMessageStreamForSession(context.Background(), "session", "must stay blocked", modelport.ChatStreamHandler{}); !errors.Is(err, ErrSessionPersistenceBlocked) {
		t.Fatalf("failed clear allowed send: %v", err)
	}
	if err := chat.RestoreSession(context.Background(), "session"); err == nil || !strings.Contains(err.Error(), "ClearSession") {
		t.Fatalf("partial clear was restored: %v", err)
	}
	// A new facade has no in-memory block reason. Real metadata must still reject restore.
	restartedMemory := callerLifecycleMemory(t)
	restarted := callerLifecycleChat(t, restartedMemory, store)
	if err := restarted.RestoreSession(context.Background(), "session"); err == nil || !strings.Contains(err.Error(), "ClearSession") {
		t.Fatalf("restart bypassed partial-clear protection: %v", err)
	}
	if store.restores != 0 {
		t.Fatal("active metadata was marked restored")
	}
	for _, event := range store.events() {
		if event == "resume" {
			t.Fatal("failed clear or active restore resumed outbox")
		}
	}
	store.saveErr = nil
	if err := restartedMemory.UseSession("other"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.ClearSession(context.Background(), "session"); err != nil {
		t.Fatal(err)
	}
	other, _ := restartedMemory.Snapshot("other")
	if restartedMemory.CurrentSessionId() != "other" || len(other.Messages) != 1 || other.Messages[0].Text() != "keep other" {
		t.Fatal("retry cleared or switched the other current session")
	}
	ctx, finish, err := restarted.beginSessionGeneration(context.Background(), "session")
	if err != nil || ctx.Err() != nil {
		t.Fatalf("successful retry left session blocked: %v", err)
	}
	finish()
}

func TestCallerRestoreResumesOnlyAfterRealDeletedMetadataTransition(t *testing.T) {
	for _, mode := range []string{"success", "store_failure", "resume_failure"} {
		t.Run(mode, func(t *testing.T) {
			memory := callerLifecycleMemory(t)
			store := &callerLifecycleStore{record: repository.SessionRecord{ID: "session", Model: "model", Deleted: true}}
			chat := callerLifecycleChat(t, memory, store)
			wantErr := errors.New(mode)
			if mode == "store_failure" {
				store.restoreErr = wantErr
			}
			if mode == "resume_failure" {
				chat.dependencies.ResumeSession = func(context.Context, string) error { store.note("resume"); return wantErr }
			}
			err := chat.RestoreSession(context.Background(), "session")
			want := []string{"block", "pause", "restore", "resume"}
			if mode == "store_failure" {
				want = want[:3]
			}
			if got := store.events(); !reflect.DeepEqual(got, want) {
				t.Fatalf("restore order=%v want=%v", got, want)
			}
			if mode == "success" {
				if err != nil || store.record.Deleted {
					t.Fatalf("restore did not update store: error=%v record=%+v", err, store.record)
				}
			} else {
				if !errors.Is(err, wantErr) {
					t.Fatalf("restore error lost: %v", err)
				}
				if store.record.Deleted != (mode == "store_failure") {
					t.Fatalf("metadata disagrees with actual completed store write: %+v", store.record)
				}
				if _, _, gateErr := chat.beginSessionGeneration(context.Background(), "session"); !errors.Is(gateErr, ErrSessionPersistenceBlocked) {
					t.Fatalf("failed restore resumed generation: %v", gateErr)
				}
			}
			if mode == "resume_failure" {
				if err := chat.RestoreSession(context.Background(), "session"); err == nil || !strings.Contains(err.Error(), "ClearSession") {
					t.Fatalf("active store was silently restored after resume failure: %v", err)
				}
			}
		})
	}
}
