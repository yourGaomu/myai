package relay

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

func TestRelayUploadAuthenticatedRoutingAndTerminalResults(t *testing.T) {
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
	token := pairTestClient(t, testServer, "123456")
	requests := []struct {
		kind      protocol.MessageType
		operation string
		payload   any
	}{
		{protocol.TypeAssetUploadBegin, "begin", protocol.AssetUploadBeginPayload{FileName: "report.txt", Size: 3, IdempotencyKey: "key"}},
		{protocol.TypeAssetUploadChunk, "chunk", protocol.AssetUploadChunkPayload{UploadID: "upload", Data: "YWJj"}},
		{protocol.TypeAssetUploadStatus, "status", protocol.AssetUploadStatusPayload{UploadID: "upload"}},
		{protocol.TypeAssetUploadComplete, "complete", protocol.AssetUploadCompletePayload{UploadID: "upload"}},
		{protocol.TypeAssetUploadCancel, "cancel", protocol.AssetUploadCancelPayload{UploadID: "upload"}},
	}
	for i, request := range requests {
		message, err := protocol.NewMessage(request.kind, fmt.Sprintf("upload-%d", i), "local", "pc-local", "session", request.payload)
		if err != nil {
			t.Fatal(err)
		}
		message.ClientToken = token
		writeTestMessage(t, clientConn, message)
		forwarded := readTestMessage(t, agentConn, request.kind)
		if forwarded.ClientToken != token || forwarded.SessionID != "session" || forwarded.RequestID != message.RequestID {
			t.Fatal("relay lost authenticated client or request identity")
		}
		readTestMessage(t, clientConn, protocol.TypeHeartbeat)
		if server.getClient(message.RequestID) == nil {
			t.Fatal("heartbeat ACK terminated upload operation before business result")
		}
		result, err := protocol.NewMessage(protocol.TypeAssetUploadResult, message.RequestID, "local", "pc-local", "session", protocol.AssetUploadResultPayload{
			Operation: request.operation, UploadID: "upload", Offset: 3,
			ChunkSize: protocol.AssetUploadChunkSize, MaxFileBytes: protocol.AssetUploadMaxFileBytes,
		})
		if err != nil {
			t.Fatal(err)
		}
		writeTestMessage(t, agentConn, result)
		readTestMessage(t, clientConn, protocol.TypeAssetUploadResult)
		readTestMessage(t, agentConn, protocol.TypeHeartbeat)
		if server.getClient(message.RequestID) != nil {
			t.Fatal("upload result retained terminal request route")
		}
		if !isTerminalResponseForRequest(request.kind, protocol.TypeAssetUploadResult) || !isTerminalResponseForRequest(request.kind, protocol.TypeError) {
			t.Fatalf("upload result or error does not terminate %s", request.kind)
		}
	}
}

func TestRelayUploadRequiresPairedIdentityAndRejectsOversizedJSON(t *testing.T) {
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
	token := pairTestClient(t, testServer, "123456")
	message, err := protocol.NewMessage(protocol.TypeAssetUploadBegin, "upload", "local", "pc-local", "session", protocol.AssetUploadBeginPayload{FileName: "x", IdempotencyKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []struct{ user, device, token string }{
		{"local", "pc-local", ""},
		{"local", "pc-local", "invalid"},
		{"other-user", "pc-local", token},
		{"local", "other-device", token},
	} {
		attempt := message
		attempt.UserID, attempt.DeviceID, attempt.ClientToken = identity.user, identity.device, identity.token
		writeTestMessage(t, clientConn, attempt)
		reply := readTestMessage(t, clientConn, protocol.TypeError)
		var payload struct {
			RequestType protocol.MessageType `json:"request_type"`
		}
		if json.Unmarshal(reply.Payload, &payload) != nil || payload.RequestType != protocol.TypeAssetUploadBegin {
			t.Fatal("upload authentication error missing request_type")
		}
	}
	message.ClientToken = token
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	// Whitespace counts toward the full JSON limit, even though it disappears
	// when Relay later remarshal forwards the decoded message.
	encoded = append(encoded, []byte(strings.Repeat(" ", protocol.AssetUploadMaxJSONBytes))...)
	if err := clientConn.WriteMessage(websocket.TextMessage, encoded); err != nil {
		t.Fatal(err)
	}
	readTestMessage(t, clientConn, protocol.TypeError)
	if server.getClient(message.RequestID) != nil {
		t.Fatal("rejected upload installed a request route")
	}
	// A valid request still works after all rejected attempts; the agent sees
	// this request first, proving no unauthorized upload was forwarded.
	writeTestMessage(t, clientConn, message)
	forwarded := readTestMessage(t, agentConn, protocol.TypeAssetUploadBegin)
	if forwarded.ClientToken != token {
		t.Fatal("authenticated token was stripped")
	}
	readTestMessage(t, clientConn, protocol.TypeHeartbeat)
	errorMessage, err := protocol.NewMessage(protocol.TypeError, message.RequestID, "local", "pc-local", "session", map[string]string{
		"message": "backend failed", "request_type": string(protocol.TypeAssetUploadBegin),
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestMessage(t, agentConn, errorMessage)
	readTestMessage(t, clientConn, protocol.TypeError)
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	if server.getClient(message.RequestID) != nil {
		t.Fatal("upload generic error did not release terminal route")
	}
}

func TestRelayUploadRouteCannotBeStolenByAnotherClient(t *testing.T) {
	server := newTestServer()
	testServer := httptest.NewServer(server.routes())
	defer testServer.Close()
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agentConn.Close()
	first := dialTestWebSocket(t, wsURL+"/ws/client")
	defer first.Close()
	second := dialTestWebSocket(t, wsURL+"/ws/client")
	defer second.Close()
	writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	firstToken := pairTestClient(t, testServer, "123456")
	// A new binding on the same authenticated agent creates a second paired
	// identity with its own token, rather than reusing the consumed bind code.
	writeAgentOnline(t, agentConn, "local", "pc-local", "654321")
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	secondToken := pairTestClient(t, testServer, "654321")
	message, err := protocol.NewMessage(protocol.TypeAssetUploadStatus, "same-request", "local", "pc-local", "session", protocol.AssetUploadStatusPayload{UploadID: "upload"})
	if err != nil {
		t.Fatal(err)
	}
	message.ClientToken = firstToken
	writeTestMessage(t, first, message)
	readTestMessage(t, agentConn, protocol.TypeAssetUploadStatus)
	readTestMessage(t, first, protocol.TypeHeartbeat)
	message.ClientToken = secondToken
	writeTestMessage(t, second, message)
	readTestMessage(t, second, protocol.TypeError)
	if route := server.getClient("same-request"); route == nil || route.ClientID != clientTokenHash(firstToken) {
		t.Fatal("another paired client stole upload route")
	}
	message.ClientToken = firstToken
	result, err := protocol.NewMessage(protocol.TypeAssetUploadResult, message.RequestID, "local", "pc-local", "session", protocol.AssetUploadResultPayload{Operation: "status", UploadID: "upload"})
	if err != nil {
		t.Fatal(err)
	}
	writeTestMessage(t, agentConn, result)
	readTestMessage(t, first, protocol.TypeAssetUploadResult)
	readTestMessage(t, agentConn, protocol.TypeHeartbeat)
	_ = second.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	var leaked protocol.Message
	if err := second.ReadJSON(&leaked); err == nil {
		t.Fatalf("upload receipt leaked to another paired client: %#v", leaked)
	}
}
