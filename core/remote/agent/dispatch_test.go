package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/adapter/chat/pendinginput"
	uuid "myai/core/adapter/id/uuid"
	messagemapper "myai/core/adapter/persistence/chatmessage/mapper"
	runmemory "myai/core/adapter/persistence/memory/agentrun"
	sessionmemory "myai/core/adapter/session/memory"
	subagentchat "myai/core/adapter/subagent/chat"
	subagentmemory "myai/core/adapter/subagent/memory"
	runservice "myai/core/application/agentrun/service"
	contextservice "myai/core/application/chat/context/service"
	generationservice "myai/core/application/chat/generation/service"
	currentservice "myai/core/application/session/current/service"
	loadservice "myai/core/application/session/load/service"
	messageservice "myai/core/application/session/message/service"
	queryservice "myai/core/application/session/query/service"
	subagentservice "myai/core/application/subagent/service"
	domainsubagent "myai/core/domain/subagent"
	modelport "myai/core/port/model"
	"myai/core/remote/protocol"
	"myai/core/service"
)

type blockingFileFacade struct {
	WorkspaceFileFacade
	started chan struct{}
	release chan struct{}
}

func (f *blockingFileFacade) Read(ctx context.Context, _ protocol.FileReadPayload) (protocol.FileReadResultPayload, error) {
	close(f.started)
	select {
	case <-f.release:
		return protocol.FileReadResultPayload{}, nil
	case <-ctx.Done():
		return protocol.FileReadResultPayload{}, ctx.Err()
	}
}

type controlChatFacade struct{ ChatFacade }

func (controlChatFacade) CurrentSessionID() string { return "session" }

func TestSlowFileReadKeepsPermissionAndPauseFast(t *testing.T) {
	facade := &blockingFileFacade{started: make(chan struct{}), release: make(chan struct{})}
	a := &Agent{chatService: controlChatFacade{}, fileService: facade, runtimes: newSessionRuntimeManager(), permissionWaiters: newPermissionWaiterRegistry()}
	turnCtx, cancel, _ := a.runtimes.get("session").start(context.Background())
	defer a.runtimes.get("session").finish(cancel)
	waiter := a.permissionWaiters.register("turn:approval")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		a.readLoop(context.Background(), conn, make(chan error, 1))
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(protocol.Message{Type: protocol.TypeFileRead, RequestID: "read", Payload: json.RawMessage(`{"path":"file"}`)}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-facade.started:
	case <-time.After(time.Second):
		t.Fatal("file read did not start")
	}
	if err := conn.WriteJSON(protocol.Message{Type: protocol.TypePermissionResult, RequestID: "turn", Payload: json.RawMessage(`{"approval_id":"approval","allowed":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(protocol.Message{Type: protocol.TypeSessionPause, RequestID: "pause", SessionID: "session", Payload: json.RawMessage(`{"session_id":"session"}`)}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waiter:
	case <-time.After(time.Second):
		t.Fatal("file read blocked permission reply")
	}
	select {
	case <-turnCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("file read blocked pause")
	}
	_ = conn.Close()
	joinCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := a.operations.wait(joinCtx); err != nil {
		t.Fatal("disconnect did not cancel facade")
	}
}

type replyIDTestModel struct {
	modelport.Registry
}

func (m replyIDTestModel) GetModel(string) modelport.ChatModelPort { return m }

func (replyIDTestModel) Generate(_ context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	request.Stream.OnReasoning("final reasoning")
	return modelport.ChatResult{Reasoning: "final reasoning"}, nil
}

func TestRemoteReplyIDsMatchHistoryAndRunSnapshots(t *testing.T) {
	const sessionID, fakeModelID = "parent", "test-model"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	memory := sessionmemory.NewStore(fakeModelID)
	if err := memory.PutSession(sessionID, fakeModelID, nil); err != nil {
		t.Fatal(err)
	}
	loader := loadservice.LoadService{Memory: memory}
	contexts := contextservice.SnapshotService{}
	runs := runmemory.New()
	pending := pendinginput.NewQueue()
	chat := service.NewChatService(service.ChatDependencies{
		Models: replyIDTestModel{}, CurrentState: currentservice.StateQueryService{Memory: memory},
		MessageCommands: messageservice.CommandService{Memory: memory, Loader: loader},
		MessageQueries:  queryservice.MessageQueryService{Memory: memory, MemoryRecords: messagemapper.Mapper{}},
		TurnInputQueue:  pending,
		AgentRunQueries: runservice.QueryService{Repository: runs},
		GenerationTasks: generationservice.TaskService{
			RequestIDs: uuid.Generator{}, Runs: runservice.CommandService{Repository: runs, IDs: uuid.Generator{}},
			Generator: generationservice.AssistantGenerationService{
				Models: replyIDTestModel{}, AgentRunner: generationservice.AgentLoopService{Sessions: memory, Contexts: contexts},
				ResponseCommitter: generationservice.ResponseCommitService{Memory: memory},
			},
		},
	})
	tasks := subagentmemory.NewRepository()
	task := domainsubagent.Task{
		ID: "child-task", ParentSessionID: sessionID, ChildSessionID: "child", CurrentRunID: "child-run",
		Status: domainsubagent.TaskStatusSucceeded, Unread: true, Result: "child evidence",
	}
	if err := tasks.SaveTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	subagents := &subagentservice.Service{Tasks: tasks, ParentContinuation: subagentchat.Continuation{Chat: chat}}
	a := &Agent{chatService: chat, subagentService: subagents, runtimes: newSessionRuntimeManager()}
	events, unsubscribe := chat.SubscribeBackgroundTurns(32)
	defer unsubscribe()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		go a.forwardBackgroundTurns(ctx, conn, events)
		// Dispatch serially so a terminal response cannot race a following
		// generation request against the runtime's cleanup.
		for {
			var message protocol.Message
			if err := conn.ReadJSON(&message); err != nil {
				return
			}
			if err := a.handleRelayMessage(ctx, conn, message); err != nil {
				t.Error(err)
				return
			}
			if err := a.waitForOperations(ctx); err != nil {
				t.Error(err)
				return
			}
		}
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetReadDeadline(deadline)

	send := func(kind protocol.MessageType, requestID, payload string) {
		t.Helper()
		if err := conn.WriteJSON(protocol.Message{Type: kind, RequestID: requestID, SessionID: sessionID, Payload: json.RawMessage(payload)}); err != nil {
			t.Fatal(err)
		}
	}
	read := func(kind protocol.MessageType, requestID string) protocol.Message {
		t.Helper()
		for {
			var message protocol.Message
			if err := conn.ReadJSON(&message); err != nil {
				t.Fatal(err)
			}
			if message.Type == protocol.TypeError {
				t.Fatalf("request %s failed: %s", message.RequestID, message.Payload)
			}
			if message.Type == kind && (requestID == "" || message.RequestID == requestID) {
				return message
			}
		}
	}
	assertReply := func(messageID, requestID string) {
		t.Helper()
		if messageID == "" || messageID == requestID {
			t.Fatalf("missing committed history ID for %s: %q", requestID, messageID)
		}
		send(protocol.TypeSessionHistory, "history", `{}`)
		history, err := protocol.DecodePayload[protocol.SessionHistoryResultPayload](read(protocol.TypeSessionHistoryResult, "history"))
		if err != nil || len(history.Messages) == 0 {
			t.Fatalf("history is unavailable: %+v / %v", history, err)
		}
		if last := history.Messages[len(history.Messages)-1]; last.ID != messageID || last.Role != "assistant" || last.Reasoning != "final reasoning" {
			t.Fatalf("reply %s did not match the assistant row: %+v", messageID, last)
		}
		send(protocol.TypeAgentRunList, "runs", `{}`)
		snapshot, err := protocol.DecodePayload[protocol.AgentRunListResultPayload](read(protocol.TypeAgentRunListResult, "runs"))
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range snapshot.Runs {
			if run.Run.RequestID != requestID {
				continue
			}
			for _, event := range run.Events {
				if event.Type == "answer" && event.MessageID == messageID && event.Content == "" {
					return
				}
			}
		}
		t.Fatalf("snapshot lacks the reasoning-only assistant identity %s for %s", messageID, requestID)
	}

	requests := []struct {
		kind    protocol.MessageType
		id      string
		payload string
	}{
		{protocol.TypeUserMessage, "send", `{"content":"hello"}`},
		{protocol.TypeSessionRegenerate, "regenerate", `{}`},
		{protocol.TypeSubagentTaskResume, "resume-parent", `{"task_id":"child-task"}`},
	}
	seen := make(map[string]bool)
	for _, request := range requests {
		if request.kind == protocol.TypeSubagentTaskResume {
			// Fill the mailbox directly so the explicit request owns the turn.
			if err := pending.EnqueueIdentified(sessionID, domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID), task.Result); err != nil {
				t.Fatal(err)
			}
		}
		send(request.kind, request.id, request.payload)
		done, err := protocol.DecodePayload[protocol.AssistantDonePayload](read(protocol.TypeAssistantDone, request.id))
		if err != nil || done.Content != "" || done.Reasoning != "final reasoning" || seen[done.MessageID] {
			t.Fatalf("invalid terminal reasoning-only reply for %s: %+v / %v", request.id, done, err)
		}
		seen[done.MessageID] = true
		assertReply(done.MessageID, request.id)
	}

	if err := chat.EnqueueQueuedUserTurn(ctx, sessionID, "queued-request", "queued question"); err != nil {
		t.Fatal(err)
	}
	for {
		event, err := protocol.DecodePayload[protocol.BackgroundTurnEventPayload](read(protocol.TypeBackgroundTurnEvent, ""))
		if err != nil {
			t.Fatal(err)
		}
		if event.Kind == "completed" {
			if event.RequestID != "queued-request" || event.Status != "succeeded" || event.Reasoning != "final reasoning" || seen[event.MessageID] {
				t.Fatalf("invalid background completion: %+v", event)
			}
			assertReply(event.MessageID, event.TurnID)
			break
		}
		if event.MessageID != "" {
			t.Fatalf("background %s received a top-level reply ID", event.Kind)
		}
	}
}

func TestOperationQueueIsBoundedOrderedAndSkipsCanceledWork(t *testing.T) {
	var queue operationQueue
	started, release := make(chan struct{}), make(chan struct{})
	if err := queue.enqueue(context.Background(), func() { close(started); <-release }); err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	for i := 0; i < operationQueueLimit; i++ {
		if err := queue.enqueue(ctx, func() { calls.Add(1) }); err != nil {
			t.Fatal(err)
		}
	}
	if err := queue.enqueue(ctx, func() {}); err == nil {
		t.Fatal("queue accepted more than its bound")
	}
	cancel()
	close(release)
	joinCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := queue.wait(joinCtx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("canceled queued work ran")
	}
	order := make(chan int, 3)
	for i := 0; i < 3; i++ {
		value := i
		if err := queue.enqueue(context.Background(), func() { order <- value }); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if actual := <-order; actual != i {
			t.Fatalf("got %d want %d", actual, i)
		}
	}
}

func TestActiveOperationAdmissionAndShutdownJoin(t *testing.T) {
	a := &Agent{}
	a.activeOperations.Store(asyncOperationLimit)
	if err := a.startAsyncOperation(context.Background(), nil, protocol.Message{}, func(context.Context, *websocket.Conn, protocol.Message) {}); err == nil {
		t.Fatal("active bound was ignored")
	}
	a.activeOperations.Store(0)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	if err := a.startAsyncOperation(ctx, nil, protocol.Message{}, func(ctx context.Context, _ *websocket.Conn, _ protocol.Message) { <-ctx.Done(); close(finished) }); err != nil {
		t.Fatal(err)
	}
	cancel()
	joinCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := a.waitForOperations(joinCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("join returned before task finished")
	}
}
