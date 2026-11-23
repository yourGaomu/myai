package relay

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myai/core/remote/protocol"
)

func testMCPDetailMessage(t *testing.T, kind protocol.MessageType, id, session string, payload any) protocol.Message {
	t.Helper()
	message, err := protocol.NewMessage(kind, id, "local", "pc-local", session, payload)
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func testMCPDetailPayload(serverName string) protocol.MCPDetailResultPayload {
	return protocol.MCPDetailResultPayload{
		Server:          protocol.MCPServerSummary{Name: serverName, State: "loaded", InventoryToolCount: 1, ExposedToolCount: 1},
		CatalogRevision: 2, Tools: []protocol.MCPToolDescriptor{{
			ID: "tool-id", Name: "read", Input: protocol.MCPSchemaSummary{State: "empty"},
			Output: protocol.MCPSchemaSummary{State: "undeclared"}, Eligibility: "not_evaluated",
		}},
	}
}

func TestRelayMCPDetailCorrelatesAuthenticatedOrigin(t *testing.T) {
	relay := newTestServer()
	server := httptest.NewServer(relay.routes())
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	agent := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agent.Close()
	owner := dialTestWebSocket(t, wsURL+"/ws/client")
	defer owner.Close()
	another := dialTestWebSocket(t, wsURL+"/ws/client")
	defer another.Close()
	writeAgentOnline(t, agent, "local", "pc-local", "123456")
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	token := pairTestClient(t, server, "123456")
	request := testMCPDetailMessage(t, protocol.TypeMCPDetail, "mobile-id", "explicit-session", protocol.MCPDetailPayload{ServerName: "demo", Cursor: "opaque"})
	expectError := func(sent protocol.Message) {
		t.Helper()
		writeTestMessage(t, owner, sent)
		failure := readTestMessage(t, owner, protocol.TypeError)
		if failure.RequestID != request.RequestID || controlReplyID(failure, "request_type") != "mcp_detail" {
			t.Fatalf("unexpected client error %+v", failure)
		}
	}
	expectError(request)
	request.ClientToken = token
	wrongTarget := request
	wrongTarget.DeviceID = "other-device"
	expectError(wrongTarget)
	tooLarge := request
	tooLarge.Payload = []byte(`{"server_name":"demo","cursor":"` + strings.Repeat("x", protocol.MCPDetailRequestMaxJSONBytes) + `"}`)
	expectError(tooLarge)
	if relay.getClient(request.RequestID) != nil {
		t.Fatal("invalid request created a route")
	}
	writeTestMessage(t, owner, request)
	forwarded := readTestMessage(t, agent, protocol.TypeMCPDetail)
	ack := readTestMessage(t, owner, protocol.TypeHeartbeat)
	if ack.RequestID != request.RequestID || forwarded.RequestID == request.RequestID || forwarded.SessionID != request.SessionID || forwarded.ClientToken != token || string(forwarded.Payload) != string(request.Payload) {
		t.Fatalf("wrong correlation or forwarding scope: %+v / %+v", ack, forwarded)
	}
	writeTestMessage(t, another, request)
	duplicate := readTestMessage(t, another, protocol.TypeError)
	if duplicate.RequestID != request.RequestID {
		t.Fatal("duplicate did not receive its own error")
	}
	validateUnsettled := func(m protocol.Message, ackType protocol.MessageType) {
		t.Helper()
		writeTestMessage(t, agent, m)
		readTestMessage(t, agent, ackType)
		if route := relay.getClient(request.RequestID); route == nil || route.detailAgentID != forwarded.RequestID {
			t.Fatal("invalid reply settled or replaced the route")
		}
	}
	makeReply := func(kind protocol.MessageType, session string, payload any) protocol.Message {
		return testMCPDetailMessage(t, kind, forwarded.RequestID, session, payload)
	}
	valid := makeReply(protocol.TypeMCPDetailResult, request.SessionID, testMCPDetailPayload("demo"))
	wrongID := valid
	wrongID.RequestID = request.RequestID
	validateUnsettled(wrongID, protocol.TypeHeartbeat)
	wrongAgentTarget := valid
	wrongAgentTarget.DeviceID = "other-device"
	validateUnsettled(wrongAgentTarget, protocol.TypeError)
	validateUnsettled(makeReply(protocol.TypeMCPDetailResult, "wrong-session", testMCPDetailPayload("demo")), protocol.TypeHeartbeat)
	validateUnsettled(makeReply(protocol.TypeMCPDetailResult, request.SessionID, testMCPDetailPayload("different-server")), protocol.TypeHeartbeat)
	validateUnsettled(makeReply(protocol.TypeMCPListResult, request.SessionID, nil), protocol.TypeError)
	validateUnsettled(makeReply(protocol.TypeError, request.SessionID, protocol.MCPDetailErrorPayload{ErrorPayload: protocol.ErrorPayload{Message: "unexpected"}, RequestType: protocol.TypeMCPList}), protocol.TypeError)
	validateUnsettled(makeReply(protocol.TypeError, request.SessionID, protocol.MCPDetailErrorPayload{ErrorPayload: protocol.ErrorPayload{Message: "unexpected"}, RequestType: protocol.TypeMCPDetail, ServerName: "different-server"}), protocol.TypeHeartbeat)
	oversize := testMCPDetailPayload("demo")
	oversize.Tools[0].Description = strings.Repeat("x", protocol.MCPDetailResponseMaxBytes)
	validateUnsettled(makeReply(protocol.TypeMCPDetailResult, request.SessionID, oversize), protocol.TypeError)

	writeTestMessage(t, agent, valid)
	got := readTestMessage(t, owner, protocol.TypeMCPDetailResult)
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	if got.RequestID != request.RequestID || got.SessionID != request.SessionID || got.ClientToken != "" || relay.getClient(request.RequestID) != nil {
		t.Fatalf("valid detail did not complete the original request: %+v", got)
	}

	writeTestMessage(t, another, request) // client request ID now available; private correlation must differ
	second := readTestMessage(t, agent, protocol.TypeMCPDetail)
	readTestMessage(t, another, protocol.TypeHeartbeat)
	if second.RequestID == forwarded.RequestID {
		t.Fatal("Relay reused a correlation ID")
	}
	writeTestMessage(t, agent, valid)
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	if relay.getClient(request.RequestID) == nil {
		t.Fatal("late result settled the second route")
	}
	stale := testMCPDetailMessage(t, protocol.TypeError, second.RequestID, request.SessionID, protocol.MCPDetailErrorPayload{
		ErrorPayload: protocol.ErrorPayload{Message: "refresh the catalog", Code: protocol.MCPErrorCatalogChanged},
		RequestType:  protocol.TypeMCPDetail, ServerName: "demo",
	})
	writeTestMessage(t, agent, stale)
	got = readTestMessage(t, another, protocol.TypeError)
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	failure, err := protocol.MCPDetailError(got)
	if err != nil || got.RequestID != request.RequestID || got.SessionID != request.SessionID || failure.Code != protocol.MCPErrorCatalogChanged || relay.getClient(request.RequestID) != nil {
		t.Fatalf("typed stale error did not complete request: %+v %+v (%v)", got, failure, err)
	}
}

func TestRelayMCPDetailRoutesExpireAndCannotMove(t *testing.T) {
	relay := newTestServer()
	first, replacement, originalAgent, newAgent := new(peer), new(peer), new(peer), new(peer)
	request := testMCPDetailMessage(t, protocol.TypeMCPDetail, "detail", "", protocol.MCPDetailPayload{ServerName: "demo"})
	request.ClientToken = "paired-client"
	relay.registerClientConnection(first, request.UserID, request.DeviceID, request.ClientToken, "first")
	original, err := relay.registerMCPDetail(first, originalAgent, "first", request, "demo")
	if err != nil {
		t.Fatal(err)
	}
	relay.unregisterClientPeer(first)
	relay.registerClientConnection(replacement, request.UserID, request.DeviceID, request.ClientToken, "replacement")
	if _, err := relay.registerMCPDetail(replacement, originalAgent, "replacement", request, "demo"); err == nil {
		t.Fatal("replacement reused pending ID")
	}
	if relay.rebindClientRequest(request.RequestID, request.UserID, request.DeviceID, request.ClientToken, replacement) {
		t.Fatal("replacement rebound the original route")
	}
	if relay.registerClient(request.RequestID, protocol.TypeMCPList, replacement, request.UserID, request.DeviceID, request.ClientToken, "replacement") {
		t.Fatal("generic request overwrote detail route")
	}
	result := testMCPDetailMessage(t, protocol.TypeMCPDetailResult, original.detailAgentID, "", testMCPDetailPayload("demo"))
	if _, target, _ := relay.clientPeerForMessage(request, nil); target != nil {
		t.Fatal("generic routing selected a connection for detail")
	}
	if err := relay.forwardMCPDetail(newAgent, result); err != nil || relay.getClient(request.RequestID) == nil {
		t.Fatal("a replacement Agent settled the original route")
	}
	if err := relay.forwardMCPDetail(originalAgent, result); err != nil || relay.getClient(request.RequestID) != nil {
		t.Fatal("disconnected requester result was not dropped and retired")
	}
	original, err = relay.registerMCPDetail(replacement, newAgent, "replacement", request, "demo")
	if err != nil {
		t.Fatal(err)
	}
	original.ConnectedAt = time.Now().Add(-mcpDetailRouteTTL)
	relay.touchClient(request.RequestID)
	relay.touchClientPeer(replacement)
	request.ClientToken = "different-paired-client"
	next, err := relay.registerMCPDetail(replacement, newAgent, "replacement", request, "demo")
	if err != nil || next.detailAgentID == original.detailAgentID {
		t.Fatalf("expired route retained or correlation reused: current=%+v (%v)", next, err)
	}
	result.RequestID = original.detailAgentID
	if err := relay.forwardMCPDetail(newAgent, result); err != nil || relay.getClient(request.RequestID) == nil {
		t.Fatal("expired reply settled a different client's route")
	}
	for i := 1; i < maxMCPDetailsPerClient; i++ {
		request.RequestID += "x"
		if _, err := relay.registerMCPDetail(new(peer), newAgent, "replacement", request, "demo"); err != nil {
			t.Fatalf("unexpected limit before %d pending requests: %v", maxMCPDetailsPerClient, err)
		}
	}
	request.RequestID += "x"
	if _, err := relay.registerMCPDetail(new(peer), newAgent, "replacement", request, "demo"); err == nil {
		t.Fatal("client exceeded in-flight cap across multiple sockets")
	}
}
