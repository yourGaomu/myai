package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/port/files"
	"myai/core/remote/protocol"
)

type uploadServiceStub struct {
	files.Service
	calls   atomic.Int32
	upload  func(context.Context, files.UploadRequest) (files.Asset, error)
	started chan struct{}
	exited  chan struct{}
}

func (s *uploadServiceStub) Upload(ctx context.Context, request files.UploadRequest) (files.Asset, error) {
	s.calls.Add(1)
	if s.started != nil {
		close(s.started)
	}
	if s.exited != nil {
		defer close(s.exited)
	}
	if s.upload != nil {
		return s.upload(ctx, request)
	}
	_, err := io.Copy(io.Discard, request.Reader)
	return files.Asset{AssetID: "asset-1", BusinessID: "business-1", Checksum: "checksum"}, err
}

type uploadChatFacade struct {
	ChatFacade
	queued chan string
}

func (f uploadChatFacade) CurrentSessionID() string { return "session" }
func (f uploadChatFacade) EnqueueTurnInput(_ string, content string) error {
	f.queued <- content
	return nil
}

func testUploadAdmission(_ context.Context, session string) (uint64, error) {
	if session != "session" {
		return 0, errors.New("unknown upload session")
	}
	return 7, nil
}

func openUploadAgent(t *testing.T, a *Agent) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer close(done)
		defer conn.Close()
		a.readLoop(ctx, conn, make(chan error, 1))
	}))
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		cancel()
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = conn.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("agent read loop did not exit")
		}
		if err := a.closeFileUploads(); err != nil {
			t.Error(err)
		}
		joinCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		if err := a.waitForOperations(joinCtx); err != nil {
			t.Error(err)
		}
		server.Close()
	})
	return conn
}

func writeUploadAgentMessage(t *testing.T, conn *websocket.Conn, kind protocol.MessageType, request string, payload any) {
	t.Helper()
	message, err := protocol.NewMessage(kind, request, "user", "device", "session", payload)
	if err != nil {
		t.Fatal(err)
	}
	message.ClientToken = "paired-token"
	if err := conn.WriteJSON(message); err != nil {
		t.Fatal(err)
	}
}

func readUploadAgentMessage(t *testing.T, conn *websocket.Conn) protocol.Message {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var message protocol.Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	return message
}

func readUploadResult(t *testing.T, conn *websocket.Conn, request, operation string) protocol.AssetUploadResultPayload {
	t.Helper()
	message := readUploadAgentMessage(t, conn)
	if message.Type != protocol.TypeAssetUploadResult || message.RequestID != request || message.SessionID != "session" {
		t.Fatalf("wrong upload response identity: %#v", message)
	}
	payload, err := protocol.DecodePayload[protocol.AssetUploadResultPayload](message)
	if err != nil || payload.Operation != operation || payload.ChunkSize != 262144 || payload.MaxFileBytes != 52428800 {
		t.Fatalf("wrong upload response contract: %#v %v", payload, err)
	}
	return payload
}

func stageAgentUpload(t *testing.T, conn *websocket.Conn, content []byte) string {
	t.Helper()
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadBegin, "begin", protocol.AssetUploadBeginPayload{
		FileName: "report.txt", ContentType: "text/plain", Size: int64(len(content)), IdempotencyKey: "key",
	})
	begun := readUploadResult(t, conn, "begin", "begin")
	if len(content) > 0 {
		writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadChunk, "chunk", protocol.AssetUploadChunkPayload{UploadID: begun.UploadID, Data: base64.StdEncoding.EncodeToString(content)})
		chunk := readUploadResult(t, conn, "chunk", "chunk")
		if chunk.Offset != int64(len(content)) {
			t.Fatal("chunk returned wrong next offset")
		}
	}
	return begun.UploadID
}

func TestAgentUploadReceiptRetryAndRecorderStableIdentity(t *testing.T) {
	service := &uploadServiceStub{}
	var recordCalls atomic.Int32
	recordedRequest := make(chan string, 2)
	a := New(Config{UserID: "user", DeviceID: "device"}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		WithFileService(service), WithFileUploadAdmission(testUploadAdmission), WithFileAssetRecorder(func(_ context.Context, session, request string, epoch uint64, asset files.Asset) error {
			if session != "session" || epoch != 7 || asset.AssetID != "asset-1" {
				t.Error("recorder lost session or asset identity")
			}
			recordedRequest <- request
			if recordCalls.Add(1) == 1 {
				return errors.New("temporary recorder error")
			}
			return nil
		}))
	conn := openUploadAgent(t, a)
	id := stageAgentUpload(t, conn, []byte("content"))
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadComplete, "first-complete", protocol.AssetUploadCompletePayload{UploadID: id})
	failure := readUploadAgentMessage(t, conn)
	if failure.Type != protocol.TypeError || failure.RequestID != "first-complete" {
		t.Fatal("recorder error was not associated with completion request")
	}
	var errorPayload struct {
		RequestType protocol.MessageType `json:"request_type"`
	}
	if json.Unmarshal(failure.Payload, &errorPayload) != nil || errorPayload.RequestType != protocol.TypeAssetUploadComplete {
		t.Fatal("generic completion error missing request_type")
	}
	for _, request := range []string{"retry-complete", "saved-receipt"} {
		writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadComplete, request, protocol.AssetUploadCompletePayload{UploadID: id})
		result := readUploadResult(t, conn, request, "complete")
		if result.Asset == nil || result.Asset.AssetID != "asset-1" || result.Asset.ShortURL != "" || result.Asset.BusinessID != "business-1" || result.Asset.Checksum != "checksum" {
			t.Fatalf("wrong uploaded asset receipt: %#v", result.Asset)
		}
	}
	if service.calls.Load() != 1 || recordCalls.Load() != 2 {
		t.Fatal("completion retry repeated upload or recording")
	}
	if first, retry := <-recordedRequest, <-recordedRequest; first != "first-complete" || retry != first {
		t.Fatalf("recorder identity changed across retries: %s %s", first, retry)
	}
}

func TestAgentUploadAdmissionFailsClosedAndRejectsChangedEpoch(t *testing.T) {
	service := &uploadServiceStub{}
	a := New(Config{UserID: "user", DeviceID: "device"}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, WithFileService(service))
	message, err := protocol.NewMessage(protocol.TypeAssetUploadBegin, "begin", "user", "device", "session", protocol.AssetUploadBeginPayload{
		FileName: "report.txt", Size: 1, IdempotencyKey: "epoch-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	message.ClientToken = "paired-token"
	if err := a.handleAssetUpload(context.Background(), nil, message); err == nil || !strings.Contains(err.Error(), "admission") {
		t.Fatalf("upload begin did not fail closed without admission: %v", err)
	}
	var currentEpoch atomic.Uint64
	currentEpoch.Store(31)
	var admissions atomic.Int32
	WithFileUploadAdmission(func(_ context.Context, session string) (uint64, error) {
		if session != "session" {
			return 0, errors.New("wrong session admission")
		}
		admissions.Add(1)
		return currentEpoch.Load(), nil
	})(a)
	conn := openUploadAgent(t, a)
	id := stageAgentUpload(t, conn, []byte("x"))
	currentEpoch.Add(1)
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadBegin, "begin-after-clear", protocol.AssetUploadBeginPayload{
		FileName: "report.txt", ContentType: "text/plain", Size: 1, IdempotencyKey: "key",
	})
	response := readUploadAgentMessage(t, conn)
	if response.Type != protocol.TypeError || response.RequestID != "begin-after-clear" || admissions.Load() != 2 {
		t.Fatal("begin retry reused an upload from another admitted session epoch")
	}
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadStatus, "status-original", protocol.AssetUploadStatusPayload{UploadID: id})
	status := readUploadResult(t, conn, "status-original", "status")
	if status.UploadID != id || service.calls.Load() != 0 {
		t.Fatal("epoch mismatch created a new backend operation")
	}
}

func TestAgentUploadOwnerIncludesAuthenticatedPairClientAndSession(t *testing.T) {
	a := New(Config{UserID: "user", DeviceID: "device"}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, WithFileService(&uploadServiceStub{}), WithFileUploadAdmission(testUploadAdmission))
	conn := openUploadAgent(t, a)
	id := stageAgentUpload(t, conn, []byte("x"))
	message, err := protocol.NewMessage(protocol.TypeAssetUploadStatus, "status", "user", "device", "session", protocol.AssetUploadStatusPayload{UploadID: id})
	if err != nil {
		t.Fatal(err)
	}
	message.ClientToken = "paired-token"
	for _, change := range []func(*protocol.Message){
		func(m *protocol.Message) { m.UserID = "other" },
		func(m *protocol.Message) { m.DeviceID = "other" },
		func(m *protocol.Message) { m.SessionID = "other" },
		func(m *protocol.Message) { m.ClientToken = "another-paired-token" },
		func(m *protocol.Message) { m.ClientToken = "" },
	} {
		wrong := message
		change(&wrong)
		if err := conn.WriteJSON(wrong); err != nil {
			t.Fatal(err)
		}
		if response := readUploadAgentMessage(t, conn); response.Type != protocol.TypeError {
			t.Fatalf("cross-owner upload was accepted: %#v", response)
		}
	}
	if err := conn.WriteJSON(message); err != nil {
		t.Fatal(err)
	}
	readUploadResult(t, conn, "status", "status")
}

func TestSlowUploadDoesNotBlockChatPermissionPauseOrStatus(t *testing.T) {
	started, exited := make(chan struct{}), make(chan struct{})
	service := &uploadServiceStub{started: started, exited: exited, upload: func(ctx context.Context, _ files.UploadRequest) (files.Asset, error) {
		<-ctx.Done()
		return files.Asset{}, ctx.Err()
	}}
	chat := uploadChatFacade{queued: make(chan string, 1)}
	a := New(Config{UserID: "user", DeviceID: "device"}, chat, nil, nil, nil, nil, nil, nil, nil, nil, nil, WithFileService(service), WithFileUploadAdmission(testUploadAdmission))
	turnCtx, cancel, _ := a.runtimes.get("session").start(context.Background())
	defer a.runtimes.get("session").finish(cancel)
	permission := a.permissionWaiters.register("turn:approval")
	conn := openUploadAgent(t, a)
	id := stageAgentUpload(t, conn, []byte("content"))
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadComplete, "complete", protocol.AssetUploadCompletePayload{UploadID: id})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("upload did not start")
	}
	writeUploadAgentMessage(t, conn, protocol.TypeUserMessage, "chat", protocol.UserMessagePayload{Content: "queued chat"})
	select {
	case content := <-chat.queued:
		if content != "queued chat" {
			t.Fatal("wrong queued chat input")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow upload blocked chat")
	}
	writeUploadAgentMessage(t, conn, protocol.TypePermissionResult, "turn", protocol.PermissionResultPayload{ApprovalID: "approval", Allowed: true})
	writeUploadAgentMessage(t, conn, protocol.TypeSessionPause, "pause", protocol.SessionPausePayload{SessionID: "session"})
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadStatus, "status", protocol.AssetUploadStatusPayload{UploadID: id})
	seen := make(map[protocol.MessageType]bool)
	for len(seen) < 4 {
		response := readUploadAgentMessage(t, conn)
		if response.Type == protocol.TypeError {
			t.Fatalf("control request failed during slow upload: %s", response.Payload)
		}
		seen[response.Type] = true
	}
	for _, kind := range []protocol.MessageType{protocol.TypeUserMessageQueued, protocol.TypePermissionResult, protocol.TypeSessionPauseResult, protocol.TypeAssetUploadResult} {
		if !seen[kind] {
			t.Fatalf("slow upload blocked %s", kind)
		}
	}
	select {
	case <-permission:
	default:
		t.Fatal("slow upload blocked permission")
	}
	select {
	case <-turnCtx.Done():
	default:
		t.Fatal("slow upload blocked pause")
	}
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadCancel, "cancel", protocol.AssetUploadCancelPayload{UploadID: id})
	gotCancel, gotCompleteError := false, false
	for !gotCancel || !gotCompleteError {
		response := readUploadAgentMessage(t, conn)
		gotCancel = gotCancel || response.Type == protocol.TypeAssetUploadResult && response.RequestID == "cancel"
		gotCompleteError = gotCompleteError || response.Type == protocol.TypeError && response.RequestID == "complete"
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("cancel did not stop backend upload")
	}
}
