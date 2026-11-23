package relay

import (
	"bytes"
	"image"
	"image/jpeg"
	"net/http/httptest"
	"strings"
	"testing"

	"myai/core/remote/protocol"
)

func TestRelayRenameCorrelatesTheRequestAndExactSession(t *testing.T) {
	relay := newTestServer()
	server := httptest.NewServer(relay.routes())
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	agent := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agent.Close()
	client := dialTestWebSocket(t, wsURL+"/ws/client")
	defer client.Close()
	writeAgentOnline(t, agent, "local", "pc-local", "123456")
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	token := pairTestClient(t, server, "123456")

	makeMessage := func(kind protocol.MessageType, requestID, sessionID string, payload any) protocol.Message {
		t.Helper()
		message, err := protocol.NewMessage(kind, requestID, "local", "pc-local", sessionID, payload)
		if err != nil {
			t.Fatal(err)
		}
		return message
	}
	requestPayload := protocol.SessionRenamePayload{SessionID: "target", Title: "新标题"}
	for _, request := range []protocol.Message{
		makeMessage(protocol.TypeSessionRename, "missing-payload-id", "target", protocol.SessionRenamePayload{Title: requestPayload.Title}),
		makeMessage(protocol.TypeSessionRename, "mismatched-ids", "another-session", requestPayload),
		makeMessage(protocol.TypeSessionRename, "", "target", requestPayload),
	} {
		request.ClientToken = token
		writeTestMessage(t, client, request)
		failure := readTestMessage(t, client, protocol.TypeError)
		if failure.RequestID != request.RequestID || controlReplyID(failure, "request_type") != string(protocol.TypeSessionRename) || relay.getClient(request.RequestID) != nil {
			t.Fatalf("invalid rename created a route or returned mismatched error: %+v", failure)
		}
	}
	request := makeMessage(protocol.TypeSessionRename, "rename", "", requestPayload)
	request.ClientToken = token
	writeTestMessage(t, client, request)
	forwarded := readTestMessage(t, agent, protocol.TypeSessionRename)
	readTestMessage(t, client, protocol.TypeHeartbeat)
	if forwarded.RequestID != request.RequestID || forwarded.SessionID != requestPayload.SessionID || !bytes.Equal(forwarded.Payload, request.Payload) {
		t.Fatalf("wrong forwarded rename: %+v", forwarded)
	}
	resultPayload := protocol.SessionRenameResultPayload{SessionID: requestPayload.SessionID, Title: requestPayload.Title, TitleRevision: 7}
	for _, mismatch := range []protocol.Message{
		makeMessage(protocol.TypeSessionRenameResult, "wrong-request", "target", resultPayload),
		makeMessage(protocol.TypeSessionRenameResult, request.RequestID, "wrong-session", resultPayload),
		makeMessage(protocol.TypeSessionRenameResult, request.RequestID, "target", protocol.SessionRenameResultPayload{SessionID: "wrong-payload-session", Title: resultPayload.Title, TitleRevision: 7}),
		makeMessage(protocol.TypeSessionListResult, request.RequestID, "target", nil),
	} {
		writeTestMessage(t, agent, mismatch)
		if mismatch.Type == protocol.TypeSessionListResult {
			readTestMessage(t, agent, protocol.TypeError)
		} else {
			readTestMessage(t, agent, protocol.TypeHeartbeat)
		}
		if relay.getClient(request.RequestID) == nil {
			t.Fatal("mismatched reply completed the rename route")
		}
	}
	writeTestMessage(t, agent, makeMessage(protocol.TypeSessionRenameResult, request.RequestID, "target", resultPayload))
	got := readTestMessage(t, client, protocol.TypeSessionRenameResult)
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	payload, err := protocol.DecodePayload[protocol.SessionRenameResultPayload](got)
	if err != nil || payload != resultPayload || got.SessionID != resultPayload.SessionID || got.RequestID != request.RequestID || relay.getClient(request.RequestID) != nil {
		t.Fatalf("incorrect rename result: envelope=%+v payload=%+v err=%v", got, payload, err)
	}
}

func TestRelayRenameCannotFollowAReplacementConnection(t *testing.T) {
	relay := newTestServer()
	first, replacement := new(peer), new(peer)
	request, err := protocol.NewMessage(protocol.TypeSessionRename, "rename-reconnect", "local", "pc-local", "session", protocol.SessionRenamePayload{SessionID: "session", Title: "Name"})
	if err != nil {
		t.Fatal(err)
	}
	request.ClientToken = "placeholder-test-token"
	relay.registerClientConnection(first, request.UserID, request.DeviceID, request.ClientToken, "first")
	if _, err := relay.registerSessionRename(first, "first", request); err != nil {
		t.Fatal(err)
	}
	relay.unregisterClientPeer(first)
	relay.registerClientConnection(replacement, request.UserID, request.DeviceID, request.ClientToken, "replacement")
	if _, err := relay.registerSessionRename(replacement, "replacement", request); err == nil {
		t.Fatal("replacement reused a pending rename request ID")
	}
	if relay.rebindClientRequest(request.RequestID, request.UserID, request.DeviceID, request.ClientToken, replacement) {
		t.Fatal("replacement took over a rename route")
	}
	result, err := protocol.NewMessage(protocol.TypeSessionRenameResult, request.RequestID, request.UserID, request.DeviceID, request.SessionID, protocol.SessionRenameResultPayload{SessionID: request.SessionID, Title: "Name", TitleRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.forwardSessionRename(result); err != nil || relay.getClient(request.RequestID) != nil {
		t.Fatalf("late rename should be dropped and retired, err=%v", err)
	}
}

func TestRelayImagePreviewRoutesToItsAuthenticatedRequest(t *testing.T) {
	relay := newTestServer()
	server := httptest.NewServer(relay.routes())
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	agent := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agent.Close()
	client := dialTestWebSocket(t, wsURL+"/ws/client")
	defer client.Close()
	writeAgentOnline(t, agent, "local", "pc-local", "123456")
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	token := pairTestClient(t, server, "123456")

	request, err := protocol.NewMessage(protocol.TypeAssetImagePreview, "preview", "local", "pc-local", "session", protocol.AssetImagePreviewPayload{AssetID: "ast_example"})
	if err != nil {
		t.Fatal(err)
	}
	writeTestMessage(t, client, request)
	failure := readTestMessage(t, client, protocol.TypeError)
	if failure.RequestID != request.RequestID || controlReplyID(failure, "request_type") != string(protocol.TypeAssetImagePreview) || relay.getClient(request.RequestID) != nil {
		t.Fatal("unauthenticated preview created a request route")
	}

	request.ClientToken = token
	writeTestMessage(t, client, request)
	forwarded := readTestMessage(t, agent, protocol.TypeAssetImagePreview)
	readTestMessage(t, client, protocol.TypeHeartbeat)
	if forwarded.SessionID != request.SessionID || forwarded.ClientToken != token || !bytes.Equal(forwarded.Payload, request.Payload) {
		t.Fatal("Relay changed preview scope")
	}

	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewGray(image.Rect(0, 0, 3, 2)), nil); err != nil {
		t.Fatal(err)
	}
	response, err := protocol.NewMessage(protocol.TypeAssetImagePreviewResult, request.RequestID, "local", "pc-local", "different-session", protocol.AssetImagePreviewResultPayload{
		AssetID: "ast_example", Data: jpegData.Bytes(), ContentType: "image/jpeg", Width: 3, Height: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestMessage(t, agent, response)
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	if relay.getClient(request.RequestID) == nil {
		t.Fatal("wrong session caused the preview route to settle")
	}
	response.SessionID = request.SessionID
	writeTestMessage(t, agent, response)
	got := readTestMessage(t, client, protocol.TypeAssetImagePreviewResult)
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	if got.SessionID != request.SessionID || !bytes.Equal(got.Payload, response.Payload) || relay.getClient(request.RequestID) != nil {
		t.Fatal("Relay returned the wrong preview or left a completed route")
	}
}

func TestRelayPreviewRequestCannotFollowAReplacementConnection(t *testing.T) {
	relay := newTestServer()
	first, replacement := new(peer), new(peer)
	request, err := protocol.NewMessage(protocol.TypeAssetImagePreview, "preview", "local", "pc-local", "session", protocol.AssetImagePreviewPayload{AssetID: "ast_example"})
	if err != nil {
		t.Fatal(err)
	}
	request.ClientToken = "placeholder-test-token"
	relay.registerClientConnection(first, request.UserID, request.DeviceID, request.ClientToken, "first")
	if _, err := relay.registerAssetPreview(first, "first", request, "ast_example"); err != nil {
		t.Fatal(err)
	}
	relay.unregisterClientPeer(first)
	relay.registerClientConnection(replacement, request.UserID, request.DeviceID, request.ClientToken, "replacement")
	if _, err := relay.registerAssetPreview(replacement, "replacement", request, "ast_example"); err == nil {
		t.Fatal("replacement connection reused a pending preview request ID")
	}
	if relay.rebindClientRequest(request.RequestID, request.UserID, request.DeviceID, request.ClientToken, replacement) {
		t.Fatal("replacement connection took over a preview route")
	}
	if _, target, _ := relay.clientPeerForMessage(request, nil); target != nil {
		t.Fatal("generic Relay delivery selected a socket for the preview")
	}
}
