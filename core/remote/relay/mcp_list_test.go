package relay

import (
	"net/http/httptest"
	"strings"
	"testing"

	"myai/core/remote/protocol"
)

func TestMCPListRoutesOnlyToRequestOwner(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()
	owner := dialTestWebSocket(t, wsURL+"/ws/client")
	defer owner.Close()
	other := dialTestWebSocket(t, wsURL+"/ws/client")
	defer other.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	ownerCredential := pairTestClient(t, testServer, "123456")
	writeAgentOnline(t, agentConn, "local", "pc-local", "654321")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	otherCredential := pairTestClient(t, testServer, "654321")

	request := protocol.Message{Type: protocol.TypeMCPList, RequestID: "mcp-list", UserID: "local", DeviceID: "pc-local"}
	writeTestMessage(t, other, request)
	readTestMessage(t, other, protocol.TypeError)
	if server.getClient(request.RequestID) != nil {
		t.Fatal("unauthenticated request created a route")
	}
	request.ClientToken = ownerCredential
	writeTestMessage(t, owner, request)
	if message := readTestMessage(t, agentConn, protocol.TypeMCPList); message.RequestID != request.RequestID {
		t.Fatalf("unexpected forwarded request id %q", message.RequestID)
	}
	readTestMessage(t, owner, protocol.TypeHeartbeat)
	request.ClientToken = otherCredential
	writeTestMessage(t, other, request)
	readTestMessage(t, other, protocol.TypeError)

	result, err := protocol.NewMessage(protocol.TypeMCPListResult, request.RequestID, request.UserID, request.DeviceID, "", protocol.MCPListResultPayload{
		Servers: []protocol.MCPServerSummary{{Name: "short_links", State: "loaded", InventoryToolCount: 2, ExposedToolCount: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestMessage(t, agentConn, result)
	if message := readTestMessage(t, owner, protocol.TypeMCPListResult); message.RequestID != request.RequestID {
		t.Fatalf("unexpected response request id %q", message.RequestID)
	}
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	if server.getClient(request.RequestID) != nil {
		t.Fatal("MCP list reply did not retire the route")
	}

	request.Type = protocol.TypeMCPReload
	request.ClientToken = ownerCredential
	writeTestMessage(t, owner, request)
	readTestMessage(t, agentConn, protocol.TypeMCPReload)
	readTestMessage(t, owner, protocol.TypeHeartbeat)
	writeTestMessage(t, agentConn, result)
	readTestMessage(t, agentConn, protocol.TypeError)
	if route := server.getClient(request.RequestID); route == nil || route.RequestType != protocol.TypeMCPReload {
		t.Fatal("a list result altered a non-list route")
	}
}
