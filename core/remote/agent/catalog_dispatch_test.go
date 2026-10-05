package agent

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"myai/core/remote/protocol"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type blockingMCPReload struct {
	started chan struct{}
	release chan struct{}
}

func (m *blockingMCPReload) ReloadMCP(ctx context.Context) error {
	close(m.started)
	select {
	case <-m.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestReloadDoesNotBlockPermissionReply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := &blockingMCPReload{started: make(chan struct{}), release: make(chan struct{})}
	defer close(manager.release)
	agent := &Agent{mcpManager: manager, permissionWaiters: newPermissionWaiterRegistry()}
	permission := agent.permissionWaiters.register("turn")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		agent.readLoop(ctx, conn, make(chan error, 1))
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(protocol.Message{Type: protocol.TypeMCPReload, Payload: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-manager.started:
	case <-time.After(5 * time.Second):
		t.Fatal("reload did not start")
	}
	if err := conn.WriteJSON(protocol.Message{Type: protocol.TypePermissionResult, RequestID: "turn", Payload: json.RawMessage(`{"allowed":true}`)}); err != nil {
		t.Fatal(err)
	}
	select {
	case allowed := <-permission:
		if !allowed {
			t.Fatal("permission denied")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reload blocked permission reply")
	}
}

func TestCatalogMutationsKeepReceiveOrder(t *testing.T) {
	agent := &Agent{}
	gate := make(chan struct{})
	order := make(chan string, 3)
	for _, id := range []string{"reload", "disable", "enable"} {
		agent.enqueueCatalogMutation(context.Background(), nil, protocol.Message{RequestID: id}, func(_ context.Context, _ *websocket.Conn, message protocol.Message) error {
			<-gate
			order <- message.RequestID
			return nil
		})
	}
	close(gate)
	for _, expected := range []string{"reload", "disable", "enable"} {
		select {
		case actual := <-order:
			if actual != expected {
				t.Fatalf("expected %s, got %s", expected, actual)
			}
		case <-time.After(time.Second):
			t.Fatal("mutation queue stalled")
		}
	}
}
