package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/mcp"
	"myai/core/remote/protocol"
)

type mcpDetailTestFacade struct {
	detail func(serverName, sessionID, cursor string) (mcp.ServerDetail, error)
}

func (mcpDetailTestFacade) ReloadMCP(context.Context) error { return nil }

func (f mcpDetailTestFacade) MCPServerDetail(serverName, sessionID, cursor string) (mcp.ServerDetail, error) {
	return f.detail(serverName, sessionID, cursor)
}

func TestMCPDetailAgentValidatesAndSanitizesReplies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readDetail := func(serverName, sessionID, cursor string) (mcp.ServerDetail, error) {
		detail := mcp.ServerDetail{
			Server:          mcp.ServerStatus{Name: serverName, State: mcp.ServerLoaded, InventoryToolCount: 1, ExposedToolCount: 1},
			CatalogRevision: 3, SessionEvaluated: sessionID == "current",
			Tools: []mcp.ToolDescriptor{{ID: "id", Name: "tool", Input: mcp.SchemaSummary{State: "empty"}, Output: mcp.SchemaSummary{State: "unsupported"}, Eligibility: "available"}},
		}
		switch cursor {
		case "stale":
			return mcp.ServerDetail{}, fmt.Errorf("private cursor context: %w", mcp.ErrCatalogChanged)
		case "missing":
			return mcp.ServerDetail{}, fmt.Errorf("private config: %w", mcp.ErrCatalogServerNotFound)
		case "invalid":
			return mcp.ServerDetail{}, mcp.ErrCatalogInvalidCursor
		case "unexpected":
			return mcp.ServerDetail{}, errors.New("private upstream error")
		case "size":
			// Marshal expands each rune to six bytes: enforce the encoded size.
			detail.Tools[0].Description = strings.Repeat("<", protocol.MCPDetailResponseMaxBytes/6)
		case "count":
			tool := detail.Tools[0]
			for range protocol.MCPDetailMaxTools {
				detail.Tools = append(detail.Tools, tool)
			}
		case "malformed":
			detail.Tools[0].Output.State = "private unsupported shape"
		case "different-server":
			detail.Server.Name = "elsewhere"
		}
		return detail, nil
	}
	a := &Agent{config: Config{UserID: "user", DeviceID: "device"}, mcpManager: mcpDetailTestFacade{detail: readDetail}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			defer conn.Close()
			a.readLoop(ctx, conn, make(chan error, 1))
		}
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	read := func() protocol.Message {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var response protocol.Message
		if err := conn.ReadJSON(&response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	request := func(cursor, token string) protocol.Message {
		t.Helper()
		message, err := protocol.NewMessage(protocol.TypeMCPDetail, "request", "user", "device", "current", protocol.MCPDetailPayload{ServerName: "fixture", Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		message.ClientToken = token
		if err := conn.WriteJSON(message); err != nil {
			t.Fatal(err)
		}
		return read()
	}

	unknown := request("", "")
	if failure, err := protocol.MCPDetailError(unknown); err != nil || !strings.Contains(failure.Message, "paired client") {
		t.Fatalf("missing paired identity was accepted: %+v (%v)", unknown, err)
	}
	for _, tc := range []struct{ cursor, message, code string }{
		{"stale", "mcp catalog changed; refresh and retry", protocol.MCPErrorCatalogChanged},
		{"missing", "mcp server was not found", ""},
		{"invalid", "invalid mcp catalog cursor", ""},
		{"unexpected", "mcp detail is unavailable", ""},
		{"size", "mcp detail is unavailable", ""},
		{"count", "mcp detail is unavailable", ""},
		{"malformed", "mcp detail is unavailable", ""},
		{"different-server", "mcp detail is unavailable", ""},
	} {
		response := request(tc.cursor, "paired-token")
		failure, err := protocol.MCPDetailError(response)
		if err != nil || failure.Message != tc.message || failure.Code != tc.code || failure.ServerName != "fixture" || response.SessionID != "current" {
			t.Errorf("cursor %q: unexpected error %+v (%v)", tc.cursor, response, err)
		}
	}

	raw := `{"type":"mcp_detail","request_id":"too-large","user_id":"user","device_id":"device","client_token":"paired-token","payload":{"server_name":"fixture"}` + strings.Repeat(" ", protocol.MCPDetailRequestMaxJSONBytes) + `}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(raw)); err != nil {
		t.Fatal(err)
	}
	tooLarge := read()
	if failure, err := protocol.MCPDetailError(tooLarge); err != nil || failure.Message != "mcp detail request is too large" {
		t.Fatalf("oversized envelope was accepted: %+v (%v)", tooLarge, err)
	}

	response := request("", "paired-token")
	payload, err := protocol.MCPDetailResult(response)
	if err != nil || response.SessionID != "current" || !payload.SessionEvaluated || len(payload.Tools) != 1 || payload.Tools[0].Eligibility != "available" {
		t.Fatalf("valid request after errors did not complete: %+v (%v)", response, err)
	}
}
