package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	memoryauthorization "myai/core/adapter/authorization/memory"
	"myai/core/remote/protocol"
)

func newTestServer() *Server {
	return NewServer(
		"",
		memoryauthorization.NewStore(),
		WithAgentCredentials(AgentCredential{UserID: "local", DeviceID: "pc-local", Token: "test-agent-token"}),
		WithAllowedOrigins("http://localhost:19006"),
	)
}

func TestAgentRunMessagesDoNotFinishChatRoute(t *testing.T) {
	if isTerminalResponseForRequest(protocol.TypeUserMessage, protocol.TypeAgentRunStarted) {
		t.Fatal("agent_run_started must not finish a chat request")
	}
	if isTerminalResponseForRequest(protocol.TypeUserMessage, protocol.TypeAgentRunCompleted) {
		t.Fatal("agent_run_completed must not finish a chat request before assistant_done")
	}
	if !isTerminalResponseForRequest(protocol.TypeUserMessage, protocol.TypeUserMessageQueued) {
		t.Fatal("user_message_queued must finish a steered chat request")
	}
	if !isTerminalResponseForRequest(protocol.TypeAgentRunList, protocol.TypeAgentRunListResult) {
		t.Fatal("agent_run_list_result must finish an agent_run_list request")
	}
}

func TestAgentRegistrationSupersedesPreviousPeer(t *testing.T) {
	server := newTestServer()
	previous := &peer{}
	current := &peer{}
	if superseded := server.registerAgent(previous, "local", "pc-local", "111111", "old"); superseded != nil {
		t.Fatalf("first registration superseded unexpected peer: %#v", superseded)
	}
	if superseded := server.registerAgent(current, "local", "pc-local", "222222", "new"); superseded != previous {
		t.Fatalf("expected previous peer to be superseded, got %#v", superseded)
	}
	if server.isCurrentAgentPeer(previous, "local", "pc-local") || !server.isCurrentAgentPeer(current, "local", "pc-local") {
		t.Fatal("registry did not retain only the latest agent peer")
	}
	server.unregisterAgent(previous, "local", "pc-local")
	if !server.isCurrentAgentPeer(current, "local", "pc-local") {
		t.Fatal("superseded disconnect removed the current agent")
	}
}

func TestRelayForwardsClientAndAgentMessages(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	clientToken := pairTestClient(t, testServer, "123456")
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeUserMessage,
		RequestID:   "req-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedToAgent := readTestMessage(t, agentConn, protocol.TypeUserMessage)
	if forwardedToAgent.RequestID != "req-1" {
		t.Fatalf("expected request id req-1, got %s", forwardedToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeAssistantDelta,
		RequestID: "req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedToClient := readTestMessage(t, clientConn, protocol.TypeAssistantDelta)
	if forwardedToClient.RequestID != "req-1" {
		t.Fatalf("expected request id req-1, got %s", forwardedToClient.RequestID)
	}
}

func TestRelayKeepsChatRequestOpenForIntermediateSkillReload(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	clientToken := pairTestClient(t, testServer, "123456")
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeUserMessage,
		RequestID:   "chat-req-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedToAgent := readTestMessage(t, agentConn, protocol.TypeUserMessage)
	if forwardedToAgent.RequestID != "chat-req-1" {
		t.Fatalf("expected request id chat-req-1, got %s", forwardedToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSkillReloadResult,
		RequestID: "chat-req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedReloadToClient := readTestMessage(t, clientConn, protocol.TypeSkillReloadResult)
	if forwardedReloadToClient.RequestID != "chat-req-1" {
		t.Fatalf("expected request id chat-req-1, got %s", forwardedReloadToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeAssistantDone,
		RequestID: "chat-req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedDoneToClient := readTestMessage(t, clientConn, protocol.TypeAssistantDone)
	if forwardedDoneToClient.RequestID != "chat-req-1" {
		t.Fatalf("expected request id chat-req-1, got %s", forwardedDoneToClient.RequestID)
	}
}

func TestRelayPermissionResultPreservesOriginalChatRoute(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()
	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	clientToken := pairTestClient(t, testServer, "123456")

	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeUserMessage,
		RequestID:   "permission-req-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		SessionID:   "session-1",
		ClientToken: clientToken,
	})
	readTestMessage(t, agentConn, protocol.TypeUserMessage)
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypePermissionAsk,
		RequestID: "permission-req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
		SessionID: "session-1",
	})
	readTestMessage(t, clientConn, protocol.TypePermissionAsk)
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypePermissionResult,
		RequestID:   "permission-req-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		SessionID:   "session-1",
		ClientToken: clientToken,
		Payload:     json.RawMessage(`{"allowed":true}`),
	})
	readTestMessage(t, agentConn, protocol.TypePermissionResult)
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	registered := server.getClient("permission-req-1")
	if registered == nil || registered.RequestType != protocol.TypeUserMessage {
		t.Fatalf("expected original user message route, got %#v", registered)
	}

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeAssistantDone,
		RequestID: "permission-req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
		SessionID: "session-1",
	})
	readTestMessage(t, clientConn, protocol.TypeAssistantDone)
	deadline := time.Now().Add(time.Second)
	for server.getClient("permission-req-1") != nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if server.getClient("permission-req-1") != nil {
		t.Fatal("chat route was not released after assistant_done")
	}
}

func TestRelayResumesChatRouteAfterClientReconnect(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()
	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	clientToken := pairTestClient(t, testServer, "123456")
	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeUserMessage,
		RequestID:   "reconnect-chat-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})
	readTestMessage(t, agentConn, protocol.TypeUserMessage)
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)
	_ = clientConn.Close()

	// A reconnect must be able to answer a permission prompt belonging to the
	// original request, even if the old peer has already been removed.
	clientConn = dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeHeartbeat,
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypePermissionResult,
		RequestID:   "reconnect-chat-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
		Payload:     json.RawMessage([]byte("{\"allowed\":true}")),
	})
	readTestMessage(t, agentConn, protocol.TypePermissionResult)
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeAssistantDelta,
		RequestID: "reconnect-chat-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})
	readTestMessage(t, clientConn, protocol.TypeAssistantDelta)
	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeAssistantDone,
		RequestID: "reconnect-chat-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})
	readTestMessage(t, clientConn, protocol.TypeAssistantDone)
	deadline := time.Now().Add(time.Second)
	for server.getClient("reconnect-chat-1") != nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if server.getClient("reconnect-chat-1") != nil {
		t.Fatal("reconnected chat route was not released after assistant_done")
	}
}

func TestRelayDoesNotAllowDifferentIdentityToReclaimRoute(t *testing.T) {
	server := newTestServer()
	peerA := &peer{}
	peerB := &peer{}
	server.registerClient("identity-route-1", protocol.TypeUserMessage, peerA, "local", "pc-local", "client-token-a", "old")
	if server.rebindClientRequest("identity-route-1", "other", "device", "client-token-a", peerB) {
		t.Fatal("different user/device reclaimed a client route")
	}
	if server.rebindClientRequest("identity-route-1", "local", "pc-local", "client-token-b", peerB) {
		t.Fatal("different paired client reclaimed a client route")
	}
	if client := server.getClient("identity-route-1"); client == nil || client.peer != peerA {
		t.Fatal("route was changed after an unauthorized reclaim attempt")
	}
}

func TestRelaySameClientRequestRetryIsIdempotent(t *testing.T) {
	server := newTestServer()
	first := &peer{}
	second := &peer{}
	if !server.registerClient("retry-route-1", protocol.TypeUserMessage, first, "local", "pc-local", "client-token-a", "old") {
		t.Fatal("initial route registration failed")
	}
	if !server.registerClient("retry-route-1", protocol.TypeSubagentTaskMessage, second, "local", "pc-local", "client-token-a", "new") {
		t.Fatal("same-client retry should be accepted")
	}
	client := server.getClient("retry-route-1")
	if client == nil || client.peer != second || client.RequestType != protocol.TypeUserMessage {
		t.Fatalf("retry replaced route semantics: %#v", client)
	}
}

func TestRelayRejectsDuplicateRequestIDFromDifferentClient(t *testing.T) {
	server := newTestServer()
	first := &peer{}
	second := &peer{}
	if !server.registerClient("duplicate-route-1", protocol.TypeUserMessage, first, "local", "pc-local", "client-token-a", "old") {
		t.Fatal("initial route registration failed")
	}
	if server.registerClient("duplicate-route-1", protocol.TypeUserMessage, second, "local", "pc-local", "client-token-b", "new") {
		t.Fatal("different client should not overwrite an existing route")
	}
	client := server.getClient("duplicate-route-1")
	if client == nil || client.peer != first || client.ClientID != clientTokenHash("client-token-a") {
		t.Fatalf("duplicate request changed route ownership: %#v", client)
	}
}

func TestRelayDoesNotDeliverRouteToSocketWithChangedIdentity(t *testing.T) {
	server := newTestServer()
	oldPeer := &peer{}
	server.registerClient("changed-identity-route-1", protocol.TypeUserMessage, oldPeer, "local", "pc-local", "client-token-a", "old")
	server.registerClientConnection(oldPeer, "other", "other-device", "client-token-b", "new")

	if _, target, ok := server.clientPeerForMessage("changed-identity-route-1", "local", "pc-local"); ok || target != nil {
		t.Fatal("route was delivered to a socket whose identity changed")
	}
}

func TestRelayForwardsPlanExecutionUntilFinalResult(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()
	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	clientToken := pairTestClient(t, testServer, "123456")

	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSessionPlanExecute,
		RequestID:   "plan-req-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		SessionID:   "session-1",
		ClientToken: clientToken,
	})
	forwarded := readTestMessage(t, agentConn, protocol.TypeSessionPlanExecute)
	if forwarded.SessionID != "session-1" {
		t.Fatalf("expected session-1, got %s", forwarded.SessionID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSessionPlanExecuteUpdate,
		RequestID: "plan-req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
		SessionID: "session-1",
	})
	readTestMessage(t, clientConn, protocol.TypeSessionPlanExecuteUpdate)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeAssistantDone,
		RequestID: "plan-req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
		SessionID: "session-1",
	})
	readTestMessage(t, clientConn, protocol.TypeAssistantDone)
	if server.getClient("plan-req-1") == nil {
		t.Fatal("plan request closed before final result")
	}

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSessionPlanExecuteResult,
		RequestID: "plan-req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
		SessionID: "session-1",
	})
	readTestMessage(t, clientConn, protocol.TypeSessionPlanExecuteResult)
	if !isTerminalResponseForRequest(protocol.TypeSessionPlanExecute, protocol.TypeSessionPlanExecuteResult) {
		t.Fatal("plan execute result must close the request route")
	}
}

func TestRelayForwardsSubagentTaskWaitResult(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()
	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	clientToken := pairTestClient(t, testServer, "123456")
	writeTestMessage(t, clientConn, protocol.Message{
		Type: protocol.TypeSubagentTaskWait, RequestID: "wait-route-1", UserID: "local", DeviceID: "pc-local",
		ClientToken: clientToken, SessionID: "session-1",
	})
	readTestMessage(t, agentConn, protocol.TypeSubagentTaskWait)
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type: protocol.TypeSubagentTaskWaitResult, RequestID: "wait-route-1", UserID: "local", DeviceID: "pc-local", SessionID: "session-1",
	})
	if result := readTestMessage(t, clientConn, protocol.TypeSubagentTaskWaitResult); result.RequestID != "wait-route-1" {
		t.Fatalf("expected wait result request id, got %s", result.RequestID)
	}
	waitForReleasedRequest(t, server, "wait-route-1")
}

func waitForReleasedRequest(t *testing.T, server *Server, requestID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if server.getClient(requestID) == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("wait result did not release its request route")
}

func TestRelayForwardsSessionMessages(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	clientToken := pairTestClient(t, testServer, "123456")
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSessionList,
		RequestID:   "session-req-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedToAgent := readTestMessage(t, agentConn, protocol.TypeSessionList)
	if forwardedToAgent.RequestID != "session-req-1" {
		t.Fatalf("expected request id session-req-1, got %s", forwardedToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSessionListResult,
		RequestID: "session-req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedToClient := readTestMessage(t, clientConn, protocol.TypeSessionListResult)
	if forwardedToClient.RequestID != "session-req-1" {
		t.Fatalf("expected request id session-req-1, got %s", forwardedToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSessionHistory,
		RequestID:   "session-history-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedHistoryToAgent := readTestMessage(t, agentConn, protocol.TypeSessionHistory)
	if forwardedHistoryToAgent.RequestID != "session-history-1" {
		t.Fatalf("expected request id session-history-1, got %s", forwardedHistoryToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSessionHistoryResult,
		RequestID: "session-history-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedHistoryToClient := readTestMessage(t, clientConn, protocol.TypeSessionHistoryResult)
	if forwardedHistoryToClient.RequestID != "session-history-1" {
		t.Fatalf("expected request id session-history-1, got %s", forwardedHistoryToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSessionPermissionSet,
		RequestID:   "session-permission-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedPermissionToAgent := readTestMessage(t, agentConn, protocol.TypeSessionPermissionSet)
	if forwardedPermissionToAgent.RequestID != "session-permission-1" {
		t.Fatalf("expected request id session-permission-1, got %s", forwardedPermissionToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSessionPermissionSetResult,
		RequestID: "session-permission-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedPermissionToClient := readTestMessage(t, clientConn, protocol.TypeSessionPermissionSetResult)
	if forwardedPermissionToClient.RequestID != "session-permission-1" {
		t.Fatalf("expected request id session-permission-1, got %s", forwardedPermissionToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSessionModeSet,
		RequestID:   "session-mode-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedModeToAgent := readTestMessage(t, agentConn, protocol.TypeSessionModeSet)
	if forwardedModeToAgent.RequestID != "session-mode-1" {
		t.Fatalf("expected request id session-mode-1, got %s", forwardedModeToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSessionModeSetResult,
		RequestID: "session-mode-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedModeToClient := readTestMessage(t, clientConn, protocol.TypeSessionModeSetResult)
	if forwardedModeToClient.RequestID != "session-mode-1" {
		t.Fatalf("expected request id session-mode-1, got %s", forwardedModeToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSessionContextSet,
		RequestID:   "session-context-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedContextToAgent := readTestMessage(t, agentConn, protocol.TypeSessionContextSet)
	if forwardedContextToAgent.RequestID != "session-context-1" {
		t.Fatalf("expected request id session-context-1, got %s", forwardedContextToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSessionContextSetResult,
		RequestID: "session-context-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedContextToClient := readTestMessage(t, clientConn, protocol.TypeSessionContextSetResult)
	if forwardedContextToClient.RequestID != "session-context-1" {
		t.Fatalf("expected request id session-context-1, got %s", forwardedContextToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSessionCompact,
		RequestID:   "session-compact-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedCompactToAgent := readTestMessage(t, agentConn, protocol.TypeSessionCompact)
	if forwardedCompactToAgent.RequestID != "session-compact-1" {
		t.Fatalf("expected request id session-compact-1, got %s", forwardedCompactToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSessionCompactResult,
		RequestID: "session-compact-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedCompactToClient := readTestMessage(t, clientConn, protocol.TypeSessionCompactResult)
	if forwardedCompactToClient.RequestID != "session-compact-1" {
		t.Fatalf("expected request id session-compact-1, got %s", forwardedCompactToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSessionDelete,
		RequestID:   "session-delete-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedDeleteToAgent := readTestMessage(t, agentConn, protocol.TypeSessionDelete)
	if forwardedDeleteToAgent.RequestID != "session-delete-1" {
		t.Fatalf("expected request id session-delete-1, got %s", forwardedDeleteToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSessionDeleteResult,
		RequestID: "session-delete-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedDeleteToClient := readTestMessage(t, clientConn, protocol.TypeSessionDeleteResult)
	if forwardedDeleteToClient.RequestID != "session-delete-1" {
		t.Fatalf("expected request id session-delete-1, got %s", forwardedDeleteToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSessionRestore,
		RequestID:   "session-restore-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedRestoreToAgent := readTestMessage(t, agentConn, protocol.TypeSessionRestore)
	if forwardedRestoreToAgent.RequestID != "session-restore-1" {
		t.Fatalf("expected request id session-restore-1, got %s", forwardedRestoreToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSessionRestoreResult,
		RequestID: "session-restore-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedRestoreToClient := readTestMessage(t, clientConn, protocol.TypeSessionRestoreResult)
	if forwardedRestoreToClient.RequestID != "session-restore-1" {
		t.Fatalf("expected request id session-restore-1, got %s", forwardedRestoreToClient.RequestID)
	}
}

func TestRelayForwardsRAGKnowledgeAndMemoryMessages(t *testing.T) {
	testCases := []struct {
		request  protocol.MessageType
		response protocol.MessageType
	}{
		{protocol.TypeSessionRAGSet, protocol.TypeSessionRAGSetResult},
		{protocol.TypeKnowledgeCatalogList, protocol.TypeKnowledgeCatalogListResult},
		{protocol.TypeKnowledgeCategoryCreate, protocol.TypeKnowledgeCatalogMutationResult},
		{protocol.TypeKnowledgeCategoryMove, protocol.TypeKnowledgeCatalogMutationResult},
		{protocol.TypeKnowledgeCategoryDelete, protocol.TypeKnowledgeCatalogMutationResult},
		{protocol.TypeKnowledgeBaseCreate, protocol.TypeKnowledgeCatalogMutationResult},
		{protocol.TypeKnowledgeBaseUpdate, protocol.TypeKnowledgeCatalogMutationResult},
		{protocol.TypeKnowledgeBaseDelete, protocol.TypeKnowledgeCatalogMutationResult},
		{protocol.TypeKnowledgeDocumentList, protocol.TypeKnowledgeDocumentListResult},
		{protocol.TypeKnowledgeDocumentIngest, protocol.TypeKnowledgeDocumentMutationResult},
		{protocol.TypeKnowledgeDocumentRetry, protocol.TypeKnowledgeDocumentMutationResult},
		{protocol.TypeKnowledgeDocumentDelete, protocol.TypeKnowledgeDocumentMutationResult},
		{protocol.TypeKnowledgeProfileList, protocol.TypeKnowledgeProfileListResult},
		{protocol.TypeKnowledgeSearchPreview, protocol.TypeKnowledgeSearchPreviewResult},
		{protocol.TypeAIMemoryList, protocol.TypeAIMemoryListResult},
		{protocol.TypeAIMemoryCreate, protocol.TypeAIMemoryMutationResult},
		{protocol.TypeAIMemoryUpdate, protocol.TypeAIMemoryMutationResult},
		{protocol.TypeAIMemoryDelete, protocol.TypeAIMemoryMutationResult},
		{protocol.TypeAIMemoryRestore, protocol.TypeAIMemoryMutationResult},
		{protocol.TypeAIMemoryCandidateList, protocol.TypeAIMemoryCandidateListResult},
		{protocol.TypeAIMemoryCandidateApprove, protocol.TypeAIMemoryCandidateMutationResult},
		{protocol.TypeAIMemoryCandidateReject, protocol.TypeAIMemoryCandidateMutationResult},
		{protocol.TypeAIMemoryExtractionJobList, protocol.TypeAIMemoryExtractionJobListResult},
		{protocol.TypeAIMemoryExtractionJobRetry, protocol.TypeAIMemoryExtractionJobRetryResult},
	}

	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()
	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	clientToken := pairTestClient(t, testServer, "123456")

	for index, testCase := range testCases {
		requestID := fmt.Sprintf("knowledge-%d", index)
		writeTestMessage(t, clientConn, protocol.Message{
			Type:        testCase.request,
			RequestID:   requestID,
			UserID:      "local",
			DeviceID:    "pc-local",
			SessionID:   "session-1",
			ClientToken: clientToken,
		})

		forwardedRequest := readTestMessage(t, agentConn, testCase.request)
		if forwardedRequest.RequestID != requestID {
			t.Fatalf("%s request id = %s, want %s", testCase.request, forwardedRequest.RequestID, requestID)
		}
		readTestMessage(t, clientConn, protocol.TypeHeartbeat)

		writeTestMessage(t, agentConn, protocol.Message{
			Type:      testCase.response,
			RequestID: requestID,
			UserID:    "local",
			DeviceID:  "pc-local",
			SessionID: "session-1",
		})

		forwardedResponse := readTestMessage(t, clientConn, testCase.response)
		if forwardedResponse.RequestID != requestID {
			t.Fatalf("%s response id = %s, want %s", testCase.response, forwardedResponse.RequestID, requestID)
		}
		readTestMessage(t, agentConn, protocol.TypeHeartbeat)
		if !isTerminalResponseForRequest(testCase.request, testCase.response) {
			t.Fatalf("%s must be terminal for %s", testCase.response, testCase.request)
		}
	}
}

func TestSessionContextQueryResultIsTerminal(t *testing.T) {
	if !isTerminalResponseForRequest(protocol.TypeSessionContextQuery, protocol.TypeSessionContextQueryResult) {
		t.Fatal("session context query result must complete its request route")
	}
}

func TestSessionGenerationResponsesAreTerminal(t *testing.T) {
	cases := []struct {
		request  protocol.MessageType
		response protocol.MessageType
	}{
		{protocol.TypeSessionGenerationQuery, protocol.TypeSessionGenerationQueryResult},
		{protocol.TypeSessionGenerationSet, protocol.TypeSessionGenerationSetResult},
		{protocol.TypeSessionStyleSet, protocol.TypeSessionStyleSetResult},
	}
	for _, testCase := range cases {
		if !isTerminalResponseForRequest(testCase.request, testCase.response) {
			t.Fatalf("%s must complete %s", testCase.response, testCase.request)
		}
	}
}

func TestRelayForwardsModelMessages(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	clientToken := pairTestClient(t, testServer, "123456")
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeModelList,
		RequestID:   "model-list-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedListToAgent := readTestMessage(t, agentConn, protocol.TypeModelList)
	if forwardedListToAgent.RequestID != "model-list-1" {
		t.Fatalf("expected request id model-list-1, got %s", forwardedListToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeModelListResult,
		RequestID: "model-list-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedListToClient := readTestMessage(t, clientConn, protocol.TypeModelListResult)
	if forwardedListToClient.RequestID != "model-list-1" {
		t.Fatalf("expected request id model-list-1, got %s", forwardedListToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeModelSwitch,
		RequestID:   "model-switch-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedSwitchToAgent := readTestMessage(t, agentConn, protocol.TypeModelSwitch)
	if forwardedSwitchToAgent.RequestID != "model-switch-1" {
		t.Fatalf("expected request id model-switch-1, got %s", forwardedSwitchToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeModelSwitchResult,
		RequestID: "model-switch-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedSwitchToClient := readTestMessage(t, clientConn, protocol.TypeModelSwitchResult)
	if forwardedSwitchToClient.RequestID != "model-switch-1" {
		t.Fatalf("expected request id model-switch-1, got %s", forwardedSwitchToClient.RequestID)
	}
}

func TestRelayForwardsSkillMessages(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	clientToken := pairTestClient(t, testServer, "123456")
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSkillList,
		RequestID:   "skill-list-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedListToAgent := readTestMessage(t, agentConn, protocol.TypeSkillList)
	if forwardedListToAgent.RequestID != "skill-list-1" {
		t.Fatalf("expected request id skill-list-1, got %s", forwardedListToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSkillListResult,
		RequestID: "skill-list-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedListToClient := readTestMessage(t, clientConn, protocol.TypeSkillListResult)
	if forwardedListToClient.RequestID != "skill-list-1" {
		t.Fatalf("expected request id skill-list-1, got %s", forwardedListToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeSkillReload,
		RequestID:   "skill-reload-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedReloadToAgent := readTestMessage(t, agentConn, protocol.TypeSkillReload)
	if forwardedReloadToAgent.RequestID != "skill-reload-1" {
		t.Fatalf("expected request id skill-reload-1, got %s", forwardedReloadToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeSkillReloadResult,
		RequestID: "skill-reload-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedReloadToClient := readTestMessage(t, clientConn, protocol.TypeSkillReloadResult)
	if forwardedReloadToClient.RequestID != "skill-reload-1" {
		t.Fatalf("expected request id skill-reload-1, got %s", forwardedReloadToClient.RequestID)
	}
}

func TestRelayForwardsSubagentEventAfterOriginalRequestCompletes(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()
	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	clientToken := pairTestClient(t, testServer, "123456")

	writeTestMessage(t, clientConn, protocol.Message{
		Type: protocol.TypeSubagentTaskList, RequestID: "subagent-list-1",
		UserID: "local", DeviceID: "pc-local", ClientToken: clientToken,
	})
	readTestMessage(t, agentConn, protocol.TypeSubagentTaskList)
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)
	writeTestMessage(t, agentConn, protocol.Message{
		Type: protocol.TypeSubagentTaskListResult, RequestID: "subagent-list-1",
		UserID: "local", DeviceID: "pc-local",
	})
	readTestMessage(t, clientConn, protocol.TypeSubagentTaskListResult)
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	if server.getClient("subagent-list-1") != nil {
		t.Fatal("expected terminal task-list response to release its request route")
	}

	writeTestMessage(t, agentConn, protocol.Message{
		Type: protocol.TypeSubagentTaskEvent, RequestID: "background-event-1",
		UserID: "local", DeviceID: "pc-local", SessionID: "parent-session-1",
	})
	event := readTestMessage(t, clientConn, protocol.TypeSubagentTaskEvent)
	if event.RequestID != "background-event-1" || event.SessionID != "parent-session-1" {
		t.Fatalf("unexpected subagent event: %#v", event)
	}
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
}

func TestRelayKeepsSubagentResumeRouteUntilDedicatedResult(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()
	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	clientToken := pairTestClient(t, testServer, "123456")

	writeTestMessage(t, clientConn, protocol.Message{
		Type: protocol.TypeSubagentTaskResume, RequestID: "subagent-resume-1", SessionID: "parent-session-1",
		UserID: "local", DeviceID: "pc-local", ClientToken: clientToken,
	})
	readTestMessage(t, agentConn, protocol.TypeSubagentTaskResume)
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type: protocol.TypeAssistantDelta, RequestID: "subagent-resume-1",
		UserID: "local", DeviceID: "pc-local", SessionID: "parent-session-1",
	})
	readTestMessage(t, clientConn, protocol.TypeAssistantDelta)
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type: protocol.TypeAssistantDone, RequestID: "subagent-resume-1",
		UserID: "local", DeviceID: "pc-local", SessionID: "parent-session-1",
	})
	readTestMessage(t, clientConn, protocol.TypeAssistantDone)
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	registered := server.getClient("subagent-resume-1")
	if registered == nil || registered.RequestType != protocol.TypeSubagentTaskResume {
		t.Fatalf("expected resume route after assistant_done, got %#v", registered)
	}

	writeTestMessage(t, agentConn, protocol.Message{
		Type: protocol.TypeSubagentTaskResumeResult, RequestID: "subagent-resume-1",
		UserID: "local", DeviceID: "pc-local", SessionID: "parent-session-1",
	})
	readTestMessage(t, clientConn, protocol.TypeSubagentTaskResumeResult)
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	if server.getClient("subagent-resume-1") != nil {
		t.Fatal("expected dedicated resume result to release its request route")
	}
}

func TestRelayForwardsFileMessages(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	clientToken := pairTestClient(t, testServer, "123456")
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeFileList,
		RequestID:   "file-req-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedToAgent := readTestMessage(t, agentConn, protocol.TypeFileList)
	if forwardedToAgent.RequestID != "file-req-1" {
		t.Fatalf("expected request id file-req-1, got %s", forwardedToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeFileListResult,
		RequestID: "file-req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedToClient := readTestMessage(t, clientConn, protocol.TypeFileListResult)
	if forwardedToClient.RequestID != "file-req-1" {
		t.Fatalf("expected request id file-req-1, got %s", forwardedToClient.RequestID)
	}
}

func TestRelayForwardsChangeMessages(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	clientToken := pairTestClient(t, testServer, "123456")
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeChangesList,
		RequestID:   "changes-req-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedToAgent := readTestMessage(t, agentConn, protocol.TypeChangesList)
	if forwardedToAgent.RequestID != "changes-req-1" {
		t.Fatalf("expected request id changes-req-1, got %s", forwardedToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeChangesListResult,
		RequestID: "changes-req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedToClient := readTestMessage(t, clientConn, protocol.TypeChangesListResult)
	if forwardedToClient.RequestID != "changes-req-1" {
		t.Fatalf("expected request id changes-req-1, got %s", forwardedToClient.RequestID)
	}
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeChangeRevert,
		RequestID:   "changes-revert-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedRevertToAgent := readTestMessage(t, agentConn, protocol.TypeChangeRevert)
	if forwardedRevertToAgent.RequestID != "changes-revert-1" {
		t.Fatalf("expected request id changes-revert-1, got %s", forwardedRevertToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeChangeRevertResult,
		RequestID: "changes-revert-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedRevertToClient := readTestMessage(t, clientConn, protocol.TypeChangeRevertResult)
	if forwardedRevertToClient.RequestID != "changes-revert-1" {
		t.Fatalf("expected request id changes-revert-1, got %s", forwardedRevertToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeHistoryList,
		RequestID:   "history-list-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedHistoryToAgent := readTestMessage(t, agentConn, protocol.TypeHistoryList)
	if forwardedHistoryToAgent.RequestID != "history-list-1" {
		t.Fatalf("expected request id history-list-1, got %s", forwardedHistoryToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeHistoryListResult,
		RequestID: "history-list-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedHistoryToClient := readTestMessage(t, clientConn, protocol.TypeHistoryListResult)
	if forwardedHistoryToClient.RequestID != "history-list-1" {
		t.Fatalf("expected request id history-list-1, got %s", forwardedHistoryToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeHistoryDiff,
		RequestID:   "history-diff-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedHistoryDiffToAgent := readTestMessage(t, agentConn, protocol.TypeHistoryDiff)
	if forwardedHistoryDiffToAgent.RequestID != "history-diff-1" {
		t.Fatalf("expected request id history-diff-1, got %s", forwardedHistoryDiffToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeHistoryDiffResult,
		RequestID: "history-diff-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedHistoryDiffToClient := readTestMessage(t, clientConn, protocol.TypeHistoryDiffResult)
	if forwardedHistoryDiffToClient.RequestID != "history-diff-1" {
		t.Fatalf("expected request id history-diff-1, got %s", forwardedHistoryDiffToClient.RequestID)
	}

	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	writeTestMessage(t, clientConn, protocol.Message{
		Type:        protocol.TypeHistoryRevert,
		RequestID:   "history-revert-1",
		UserID:      "local",
		DeviceID:    "pc-local",
		ClientToken: clientToken,
	})

	forwardedHistoryRevertToAgent := readTestMessage(t, agentConn, protocol.TypeHistoryRevert)
	if forwardedHistoryRevertToAgent.RequestID != "history-revert-1" {
		t.Fatalf("expected request id history-revert-1, got %s", forwardedHistoryRevertToAgent.RequestID)
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)

	writeTestMessage(t, agentConn, protocol.Message{
		Type:      protocol.TypeHistoryRevertResult,
		RequestID: "history-revert-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	forwardedHistoryRevertToClient := readTestMessage(t, clientConn, protocol.TypeHistoryRevertResult)
	if forwardedHistoryRevertToClient.RequestID != "history-revert-1" {
		t.Fatalf("expected request id history-revert-1, got %s", forwardedHistoryRevertToClient.RequestID)
	}
}

func TestRelayRejectsClientWithoutToken(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	writeTestMessage(t, clientConn, protocol.Message{
		Type:      protocol.TypeUserMessage,
		RequestID: "req-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	errorMessage := readTestMessage(t, clientConn, protocol.TypeError)
	payload, err := protocol.DecodePayload[protocol.ErrorPayload](errorMessage)
	if err != nil {
		t.Fatalf("decode error payload failed: %v", err)
	}
	if payload.Message != "client token is invalid or expired" {
		t.Fatalf("unexpected error message: %s", payload.Message)
	}
}

func TestRelayRejectsAgentWebSocketWithoutSharedToken(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws/agent"
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("expected unauthenticated agent websocket to be rejected")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %#v", http.StatusUnauthorized, response)
	}
}

func TestRelayRejectsAgentCredentialForDifferentIdentity(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws/agent"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer test-agent-token")
	headers.Set(protocol.HeaderAgentUserID, "local")
	headers.Set(protocol.HeaderAgentDeviceID, "other-device")
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("expected identity-bound credential to reject a different device")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %#v", http.StatusUnauthorized, response)
	}
}

func TestRelayRejectsAgentRegistrationThatDoesNotMatchCredential(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	writeAgentOnline(t, agentConn, "other-user", "other-device", "123456")
	errorMessage := readTestMessage(t, agentConn, protocol.TypeError)
	payload, err := protocol.DecodePayload[protocol.ErrorPayload](errorMessage)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Message != "agent registration identity does not match its credential" {
		t.Fatalf("unexpected error message: %s", payload.Message)
	}
}

func TestRelayRejectsUntrustedWebSocketOrigin(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws/agent"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer test-agent-token")
	headers.Set(protocol.HeaderAgentUserID, "local")
	headers.Set(protocol.HeaderAgentDeviceID, "pc-local")
	headers.Set("Origin", "https://attacker.example")
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("expected untrusted websocket origin to be rejected")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected status %d, got %#v", http.StatusForbidden, response)
	}
}

func TestRelayAllowsAnyWebSocketOriginWhenWildcardIsConfigured(t *testing.T) {
	server := NewServer(
		"",
		memoryauthorization.NewStore(),
		WithAgentCredentials(AgentCredential{UserID: "local", DeviceID: "pc-local", Token: "test-agent-token"}),
		WithAllowedOrigins("*"),
	)
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws/agent"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer test-agent-token")
	headers.Set(protocol.HeaderAgentUserID, "local")
	headers.Set(protocol.HeaderAgentDeviceID, "pc-local")
	headers.Set("Origin", "https://mobile.example")
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Fatalf("expected wildcard origin to allow websocket connection: %v", err)
	}
	defer conn.Close()
}

func TestRelayAllowsAnyLocalExpoPortWhenLoopbackWildcardIsConfigured(t *testing.T) {
	server := NewServer(
		"",
		memoryauthorization.NewStore(),
		WithAllowedOrigins("http://localhost:*", "http://127.0.0.1:*"),
	)
	request := httptest.NewRequest(http.MethodGet, "http://relay.test/ws/client", nil)
	request.Header.Set("Origin", "http://localhost:8083")
	if !server.originAllowed(request) {
		t.Fatal("expected localhost development origin to be allowed")
	}
	request.Header.Set("Origin", "http://127.0.0.1:19006")
	if !server.originAllowed(request) {
		t.Fatal("expected loopback development origin to be allowed")
	}
	request.Header.Set("Origin", "http://192.168.1.20:8083")
	if server.originAllowed(request) {
		t.Fatal("did not expect LAN origin to match loopback wildcard")
	}
}

func TestRelayProtectsAgentInventory(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	response, err := testServer.Client().Get(testServer.URL + "/agents")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.StatusCode)
	}

	request, err := http.NewRequest(http.MethodGet, testServer.URL+"/agents", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer test-agent-token")
	request.Header.Set(protocol.HeaderAgentUserID, "local")
	request.Header.Set(protocol.HeaderAgentDeviceID, "pc-local")
	response, err = testServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.StatusCode)
	}
}

func TestRelayLimitsPairedClientInventoryToItsAgent(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()
	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	clientToken := pairTestClient(t, testServer, "123456")

	request, err := http.NewRequest(http.MethodGet, testServer.URL+"/agents?user_id=local&device_id=pc-local", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+clientToken)
	response, err := testServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.StatusCode)
	}
	var payload agentsResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Agents) != 1 || payload.Agents[0].UserID != "local" || payload.Agents[0].DeviceID != "pc-local" {
		t.Fatalf("unexpected client-visible agents: %#v", payload.Agents)
	}

	otherRequest, err := http.NewRequest(http.MethodGet, testServer.URL+"/agents?user_id=other&device_id=other", nil)
	if err != nil {
		t.Fatal(err)
	}
	otherRequest.Header.Set("Authorization", "Bearer "+clientToken)
	otherResponse, err := testServer.Client().Do(otherRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer otherResponse.Body.Close()
	if otherResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected cross-identity inventory status %d, got %d", http.StatusUnauthorized, otherResponse.StatusCode)
	}
}

func TestRelayRejectsUnsupportedClientMessage(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
	defer clientConn.Close()

	writeTestMessage(t, clientConn, protocol.Message{
		Type:      protocol.MessageType("unknown_request"),
		RequestID: "unknown-1",
		UserID:    "local",
		DeviceID:  "pc-local",
	})

	errorMessage := readTestMessage(t, clientConn, protocol.TypeError)
	payload, err := protocol.DecodePayload[protocol.ErrorPayload](errorMessage)
	if err != nil {
		t.Fatalf("decode error payload failed: %v", err)
	}
	if payload.Message != "unsupported client message type: unknown_request" {
		t.Fatalf("unexpected error message: %s", payload.Message)
	}
}

func TestRelayPairsAgentByBindCode(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	pair := pairTestResponse(t, testServer, "123456")
	if pair.UserID != "local" || pair.DeviceID != "pc-local" {
		t.Fatalf("unexpected pair response: %+v", pair)
	}
	if pair.ClientToken == "" {
		t.Fatalf("expected client token")
	}
}

func TestRelayAllowsBrowserCorsPreflight(t *testing.T) {
	server := NewServer(
		"",
		memoryauthorization.NewStore(),
		WithAgentCredentials(AgentCredential{UserID: "local", DeviceID: "pc-local", Token: "test-agent-token"}),
		WithAllowedOrigins("http://localhost:*", "http://127.0.0.1:*"),
	)
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	request, err := http.NewRequest(http.MethodOptions, testServer.URL+"/pair", nil)
	if err != nil {
		t.Fatalf("new preflight request failed: %v", err)
	}
	request.Header.Set("Origin", "http://localhost:8083")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "content-type")

	response, err := testServer.Client().Do(request)
	if err != nil {
		t.Fatalf("send preflight request failed: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.StatusCode)
	}
	if got := response.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:8083" {
		t.Fatalf("expected CORS origin http://localhost:8083, got %q", got)
	}
	if got := response.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "content-type") {
		t.Fatalf("expected CORS headers to include content-type, got %q", got)
	}
}

func TestRelayAllowsAnyCorsOriginWhenWildcardIsConfigured(t *testing.T) {
	server := NewServer(
		"",
		memoryauthorization.NewStore(),
		WithAgentCredentials(AgentCredential{UserID: "local", DeviceID: "pc-local", Token: "test-agent-token"}),
		WithAllowedOrigins("*"),
	)
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	request, err := http.NewRequest(http.MethodOptions, testServer.URL+"/pair", nil)
	if err != nil {
		t.Fatalf("new preflight request failed: %v", err)
	}
	request.Header.Set("Origin", "https://mobile.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)

	response, err := testServer.Client().Do(request)
	if err != nil {
		t.Fatalf("send preflight request failed: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.StatusCode)
	}
	if got := response.Header.Get("Access-Control-Allow-Origin"); got != "https://mobile.example" {
		t.Fatalf("expected echoed CORS origin, got %q", got)
	}
}

func TestRelayListsAndRevokesAuthorizations(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()

	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)

	first := pairTestResponse(t, testServer, "123456")
	second := pairTestResponse(t, testServer, "123456")

	auths := listTestAuthorizations(t, testServer, "local", "pc-local", first.ClientToken, http.StatusOK)
	if len(auths.Authorizations) != 2 {
		t.Fatalf("expected 2 authorizations, got %d", len(auths.Authorizations))
	}

	var other AuthorizationInfo
	for _, authorization := range auths.Authorizations {
		if !authorization.Current {
			other = authorization
			break
		}
	}
	if other.ID == "" {
		t.Fatalf("expected non-current authorization")
	}

	revokeTestAuthorization(t, testServer, "local", "pc-local", first.ClientToken, other.ID)

	auths = listTestAuthorizations(t, testServer, "local", "pc-local", first.ClientToken, http.StatusOK)
	if len(auths.Authorizations) != 1 {
		t.Fatalf("expected 1 authorization after revoke, got %d", len(auths.Authorizations))
	}
	if !auths.Authorizations[0].Current {
		t.Fatalf("expected remaining authorization to be current")
	}

	listTestAuthorizations(t, testServer, "local", "pc-local", second.ClientToken, http.StatusUnauthorized)
}

func writeAgentOnline(t *testing.T, conn *websocket.Conn, userID string, deviceID string, bindCode string) {
	t.Helper()

	message, err := protocol.NewMessage(
		protocol.TypeAgentOnline,
		"agent-online",
		userID,
		deviceID,
		"",
		protocol.AgentOnlinePayload{Status: "online", BindCode: bindCode},
	)
	if err != nil {
		t.Fatalf("new agent online message failed: %v", err)
	}
	writeTestMessage(t, conn, message)
}

func pairTestClient(t *testing.T, server *httptest.Server, bindCode string) string {
	t.Helper()

	return pairTestResponse(t, server, bindCode).ClientToken
}

func pairTestResponse(t *testing.T, server *httptest.Server, bindCode string) pairResponse {
	t.Helper()

	body, err := json.Marshal(pairRequest{BindCode: bindCode})
	if err != nil {
		t.Fatalf("marshal pair request failed: %v", err)
	}
	response, err := server.Client().Post(server.URL+"/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post pair failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}

	var pair pairResponse
	if err := json.NewDecoder(response.Body).Decode(&pair); err != nil {
		t.Fatalf("decode pair response failed: %v", err)
	}
	return pair
}

func listTestAuthorizations(t *testing.T, server *httptest.Server, userID string, deviceID string, token string, expectedStatus int) authorizationsResponse {
	t.Helper()

	values := url.Values{}
	values.Set("user_id", userID)
	values.Set("device_id", deviceID)

	request, err := http.NewRequest(http.MethodGet, server.URL+"/authorizations?"+values.Encode(), nil)
	if err != nil {
		t.Fatalf("new authorizations request failed: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("list authorizations failed: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != expectedStatus {
		t.Fatalf("expected status %d, got %d", expectedStatus, response.StatusCode)
	}
	if expectedStatus != http.StatusOK {
		return authorizationsResponse{}
	}

	var authorizations authorizationsResponse
	if err := json.NewDecoder(response.Body).Decode(&authorizations); err != nil {
		t.Fatalf("decode authorizations response failed: %v", err)
	}
	return authorizations
}

func revokeTestAuthorization(t *testing.T, server *httptest.Server, userID string, deviceID string, token string, authorizationID string) {
	t.Helper()

	body, err := json.Marshal(revokeAuthorizationRequest{
		ID:          authorizationID,
		UserID:      userID,
		DeviceID:    deviceID,
		ClientToken: token,
	})
	if err != nil {
		t.Fatalf("marshal revoke request failed: %v", err)
	}

	response, err := server.Client().Post(server.URL+"/authorizations/revoke", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("revoke authorization failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}
}

func dialTestWebSocket(t *testing.T, url string) *websocket.Conn {
	t.Helper()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer test-agent-token")
	headers.Set(protocol.HeaderAgentUserID, "local")
	headers.Set(protocol.HeaderAgentDeviceID, "pc-local")
	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		t.Fatalf("dial websocket %s failed: %v", url, err)
	}
	return conn
}

func writeTestMessage(t *testing.T, conn *websocket.Conn, message protocol.Message) {
	t.Helper()

	if err := conn.WriteJSON(message); err != nil {
		t.Fatalf("write message failed: %v", err)
	}
}

func readTestMessage(t *testing.T, conn *websocket.Conn, expectedType protocol.MessageType) protocol.Message {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set read deadline failed: %v", err)
	}

	var message protocol.Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("read message failed: %v", err)
	}
	if message.Type != expectedType {
		t.Fatalf("expected message type %s, got %s", expectedType, message.Type)
	}
	return message
}
