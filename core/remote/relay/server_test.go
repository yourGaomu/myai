package relay

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

func TestRelayForwardsClientAndAgentMessages(t *testing.T) {
	server := NewServer("")
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

func TestRelayForwardsSessionMessages(t *testing.T) {
	server := NewServer("")
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
}

func TestRelayForwardsFileMessages(t *testing.T) {
	server := NewServer("")
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
	server := NewServer("")
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
	server := NewServer("")
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

func TestRelayPairsAgentByBindCode(t *testing.T) {
	server := NewServer("")
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

func TestRelayListsAndRevokesAuthorizations(t *testing.T) {
	server := NewServer("")
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

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
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
