package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/mcp"
	"myai/core/remote/protocol"
)

type mcpListTestFacade struct {
	*blockingMCPReload
	statuses []mcp.ServerStatus
}

func (f *mcpListTestFacade) MCPServerStatuses() ([]mcp.ServerStatus, error) {
	return f.statuses, nil
}

func TestMCPListReadsPublishedCatalogDuringReload(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []mcp.ServerStatus
	}{
		{name: "short_links", statuses: []mcp.ServerStatus{{Name: "short_links", State: mcp.ServerLoaded, InventoryToolCount: 2, ExposedToolCount: 1}}},
		{name: "empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			manager := &mcpListTestFacade{
				blockingMCPReload: &blockingMCPReload{started: make(chan struct{}), release: make(chan struct{})},
				statuses:          tc.statuses,
			}
			a := &Agent{mcpManager: manager}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				a.readLoop(ctx, conn, make(chan error, 1))
			}))
			defer server.Close()
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err := conn.WriteJSON(protocol.Message{Type: protocol.TypeMCPReload, RequestID: "reload"}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-manager.started:
			case <-time.After(5 * time.Second):
				t.Fatal("reload did not start")
			}
			if err := conn.WriteJSON(protocol.Message{Type: protocol.TypeMCPList, RequestID: "list"}); err != nil {
				t.Fatal(err)
			}
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			var message protocol.Message
			if err := conn.ReadJSON(&message); err != nil {
				t.Fatal(err)
			}
			if message.Type != protocol.TypeMCPListResult || message.RequestID != "list" {
				t.Fatalf("unexpected list response: %#v", message)
			}
			payload, err := protocol.DecodePayload[protocol.MCPListResultPayload](message)
			if err != nil || payload.Servers == nil || len(payload.Servers) != len(tc.statuses) || payload.CatalogRevision != nil {
				t.Fatalf("unexpected list payload: %#v, %v", payload, err)
			}
			if len(payload.Servers) > 0 && payload.Servers[0] != (protocol.MCPServerSummary{Name: "short_links", State: "loaded", InventoryToolCount: 2, ExposedToolCount: 1}) {
				t.Fatalf("unexpected short_links entry: %#v", payload.Servers[0])
			}
			close(manager.release)
			if err := conn.ReadJSON(&message); err != nil || message.Type != protocol.TypeMCPReloadResult || message.RequestID != "reload" {
				t.Fatalf("unexpected reload response: %#v, %v", message, err)
			}
		})
	}
}
