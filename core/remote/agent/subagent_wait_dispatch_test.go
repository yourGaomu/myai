package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	subagentcommand "myai/core/application/subagent/command"
	subagentresult "myai/core/application/subagent/result"
	"myai/core/remote/protocol"
)

type blockingSubagentWait struct {
	SubagentFacade
	started chan struct{}
	stopped chan struct{}
}

func (s *blockingSubagentWait) Wait(ctx context.Context, _ subagentcommand.WaitTask) (subagentresult.Wait, error) {
	close(s.started)
	defer close(s.stopped)
	<-ctx.Done()
	return subagentresult.Wait{}, ctx.Err()
}
func TestSubagentWaitDoesNotBlockRepliesAndStopsOnDisconnect(t *testing.T) {
	svc := &blockingSubagentWait{started: make(chan struct{}), stopped: make(chan struct{})}
	a := &Agent{subagentService: svc, permissionWaiters: newPermissionWaiterRegistry()}
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
	if err := conn.WriteJSON(protocol.Message{Type: protocol.TypeSubagentTaskWait, SessionID: "session", Payload: json.RawMessage(`{"task_id":"task"}`)}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-svc.started:
	case <-time.After(time.Second):
		t.Fatal("wait did not start")
	}
	if err := conn.WriteJSON(protocol.Message{Type: protocol.TypePermissionResult, RequestID: "turn", Payload: json.RawMessage(`{"approval_id":"approval","allowed":true}`)}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waiter:
	case <-time.After(time.Second):
		t.Fatal("wait blocked permission reply")
	}
	_ = conn.Close()
	select {
	case <-svc.stopped:
	case <-time.After(time.Second):
		t.Fatal("disconnect leaked wait operation")
	}
}
