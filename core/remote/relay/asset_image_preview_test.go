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
