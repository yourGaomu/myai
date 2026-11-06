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
	"myai/core/remote/protocol"
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
