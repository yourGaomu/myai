package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	shortlinksfiles "myai/core/adapter/files/shortlinks"
	uuidadapter "myai/core/adapter/id/uuid"
	modeladapter "myai/core/adapter/model/langchaingo"
	messagemapper "myai/core/adapter/persistence/chatmessage/mapper"
	sessionmemory "myai/core/adapter/session/memory"
	contextservice "myai/core/application/chat/context/service"
	generationservice "myai/core/application/chat/generation/service"
	currentservice "myai/core/application/session/current/service"
	loadservice "myai/core/application/session/load/service"
	messageservice "myai/core/application/session/message/service"
	queryservice "myai/core/application/session/query/service"
	domainmodel "myai/core/domain/model"
	"myai/core/mcp"
	"myai/core/port/files"
	modelport "myai/core/port/model"
	"myai/core/remote/protocol"
	"myai/core/service"
	"myai/core/tool"
)

type mobileImageModelRegistry struct {
	modelport.Registry
	model modelport.ChatModelPort
}

func (m mobileImageModelRegistry) GetModel(string) modelport.ChatModelPort { return m.model }

func TestMobileJPEGUploadPreviewAndModelRequest(t *testing.T) {
	const sessionID, modelID = "session", "test-vision"
	fixture := image.NewRGBA(image.Rect(0, 0, 7, 5))
	for y := range 5 {
		for x := range 7 {
			fixture.SetRGBA(x, y, color.RGBA{R: 255, G: 40, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, fixture, nil); err != nil {
		t.Fatal(err)
	}
	original := encoded.Bytes()

	backend := &mobileMCPBackend{t: t, parts: make(map[int][]byte), calls: make(map[string]int)}
	var objectCalls atomic.Int32
	object := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		objectCalls.Add(1)
		if r.URL.Query().Get("X-Amz-Signature") != "integration-signed-secret" || r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" {
			t.Error("wrong object download authentication")
		}
		backend.mu.Lock()
		data := backend.dataLocked()
		backend.mu.Unlock()
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(data)
	}))
	t.Cleanup(object.Close)
	backend.downloadURL = object.URL + "/object?X-Amz-Signature=integration-signed-secret"
	mcpServer := httptest.NewServer(backend)
	t.Cleanup(mcpServer.Close)
	manager := mcp.NewManager(mcp.Config{Servers: []mcp.ServerConfig{{
		Name: "short_links", URL: mcpServer.URL + "/api/mcp", Required: true, TimeoutSeconds: 5,
		Headers: map[string]string{"Authorization": "Bearer mcp-integration-secret", "X-API-Key": "integration-api-secret"},
	}}})
	if err := manager.RegisterAll(t.Context(), tool.NewRegisterTools()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	fileService := shortlinksfiles.New(func() *mcp.Manager { return manager })

	var modelCalls atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || json.NewDecoder(r.Body).Decode(&request) != nil || request.Model != modelID {
			t.Error("invalid model request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		modelCalls.Add(1)
		var matchingImages int
		for _, message := range request.Messages {
			if message.Role != "user" {
				continue
			}
			var parts []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			}
			if json.Unmarshal(message.Content, &parts) != nil {
				continue
			}
			var hasQuestion bool
			var hasJPEG bool
			for _, part := range parts {
				if part.Type == "text" && strings.Contains(part.Text, "What color is this?") {
					hasQuestion = true
				}
				if part.Type == "image_url" {
					prefix, b64, ok := strings.Cut(part.ImageURL.URL, ",")
					data, err := base64.StdEncoding.DecodeString(b64)
					photo, photoErr := jpeg.Decode(bytes.NewReader(data))
					if !ok || prefix != "data:image/jpeg;base64" || err != nil || photoErr != nil || !bytes.Equal(data, original) || photo.Bounds().Dx() != 7 {
						t.Error("model did not receive the verified uploaded JPEG")
						continue
					}
					hasJPEG = true
				}
			}
			if hasQuestion && hasJPEG {
				matchingImages++
			}
		}
		if matchingImages != 1 {
			t.Errorf("want one source user message with text and JPEG, got %d", matchingImages)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"The image is red.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(modelServer.Close)
	model, err := (modeladapter.OpenAIChatCompletionsAdapter{}).CreateModel(modelport.CreationConfig{
		BaseURL: modelServer.URL + "/v1", AuthType: domainmodel.AuthTypeNone, ModelName: modelID,
	})
	if err != nil {
		t.Fatal(err)
	}
	models := mobileImageModelRegistry{model: model}
	memory := sessionmemory.NewStore(modelID)
	if err := memory.PutSession(sessionID, modelID, nil); err != nil {
		t.Fatal(err)
	}
	loader := loadservice.LoadService{Memory: memory}
	var mu sync.Mutex
	var uploaded files.Asset
	lookup := func(_ context.Context, session, assetID string) (files.Asset, error) {
		mu.Lock()
		defer mu.Unlock()
		if session != sessionID || uploaded.AssetID != assetID {
			return files.Asset{}, files.ErrSessionAssetUnavailable
		}
		return uploaded, nil
	}
	chat := service.NewChatService(service.ChatDependencies{
		Models: models, CurrentState: currentservice.StateQueryService{Memory: memory},
		MessageCommands:    messageservice.CommandService{Memory: memory, Loader: loader},
		MessageQueries:     queryservice.MessageQueryService{Memory: memory, MemoryRecords: messagemapper.Mapper{}},
		LookupSessionAsset: lookup, AssetFiles: fileService,
		GenerationTasks: generationservice.TaskService{RequestIDs: uuidadapter.Generator{}, Generator: generationservice.AssistantGenerationService{
			Models: models, AgentRunner: generationservice.AgentLoopService{
				Sessions: memory, Contexts: contextservice.SnapshotService{}, AssetFiles: fileService, LookupSessionAsset: lookup,
			}, ResponseCommitter: generationservice.ResponseCommitService{Memory: memory},
		}},
	})
	a := New(Config{UserID: "user", DeviceID: "device"}, chat, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		WithFileService(fileService), WithFileUploadAdmission(testUploadAdmission), WithFileAssetRecorder(func(_ context.Context, session, _ string, _ uint64, asset files.Asset) error {
			if session != sessionID {
				t.Error("upload recorded for another session")
			}
			mu.Lock()
			uploaded = asset
			mu.Unlock()
			return nil
		}))
	conn := openUploadAgent(t, a)
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadBegin, "begin-jpeg", protocol.AssetUploadBeginPayload{
		FileName: "screenshot.jpg", ContentType: "image/jpeg", Size: int64(len(original)), IdempotencyKey: "test-jpeg",
	})
	begun := readIntegrationUploadResult(t, conn, "begin-jpeg", "begin")
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadChunk, "chunk-jpeg", protocol.AssetUploadChunkPayload{UploadID: begun.UploadID, Data: base64.StdEncoding.EncodeToString(original)})
	readIntegrationUploadResult(t, conn, "chunk-jpeg", "chunk")
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadComplete, "complete-jpeg", protocol.AssetUploadCompletePayload{UploadID: begun.UploadID})
	completed := readIntegrationUploadResult(t, conn, "complete-jpeg", "complete")
	if completed.Asset == nil || completed.Asset.AssetID != integrationAssetID || completed.Asset.ShortURL != "" {
		t.Fatal("JPEG upload did not return a private asset")
	}
	preview := protocol.AssetImagePreviewPayload{AssetID: completed.Asset.AssetID}
	other, err := protocol.NewMessage(protocol.TypeAssetImagePreview, "other-preview", "user", "device", "different-session", preview)
	if err != nil {
		t.Fatal(err)
	}
	other.ClientToken = "paired-token"
	if err := conn.WriteJSON(other); err != nil {
		t.Fatal(err)
	}
	if denied := readUploadAgentMessage(t, conn); denied.Type != protocol.TypeError || denied.RequestID != other.RequestID || objectCalls.Load() != 0 {
		t.Fatal("cross-session preview was not rejected before fetching the object")
	}
	writeUploadAgentMessage(t, conn, protocol.TypeAssetImagePreview, "jpeg-preview", preview)
	result := readUploadAgentMessage(t, conn)
	assertIntegrationNoSecrets(t, result)
	thumbnail, err := protocol.AssetImagePreviewResult(result)
	if err != nil || result.RequestID != "jpeg-preview" || thumbnail.AssetID != integrationAssetID || thumbnail.Width != 7 || thumbnail.Height != 5 {
		t.Fatalf("invalid image preview: %v", err)
	}

	input := fmt.Sprintf("What color is this?\n\nAttached files:\n<uploaded_file asset_id=\"%s\" name=\"screenshot.jpg\" content_type=\"image/jpeg\" size=\"%d\">\n[User attachment]\n</uploaded_file>", integrationAssetID, len(original))
	for _, turn := range []struct {
		kind      protocol.MessageType
		requestID string
		payload   any
	}{
		{protocol.TypeUserMessage, "vision", protocol.UserMessagePayload{Content: input}},
		{protocol.TypeSessionRegenerate, "regenerate", protocol.SessionRegeneratePayload{SessionID: sessionID}},
		{protocol.TypeUserMessage, "follow-up", protocol.UserMessagePayload{Content: "What about the same image?"}},
	} {
		writeUploadAgentMessage(t, conn, turn.kind, turn.requestID, turn.payload)
		for {
			response := readUploadAgentMessage(t, conn)
			assertIntegrationNoSecrets(t, response)
			if response.Type == protocol.TypeError {
				t.Fatalf("%s failed: %s", turn.requestID, response.Payload)
			}
			if response.Type == protocol.TypeAssistantDone {
				done, err := protocol.DecodePayload[protocol.AssistantDonePayload](response)
				if err != nil || done.Content != "The image is red." || response.RequestID != turn.requestID {
					t.Fatalf("unexpected model answer for %s: %v", turn.requestID, err)
				}
				break
			}
		}
		if err := a.waitForOperations(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if modelCalls.Load() != 3 || objectCalls.Load() != 4 {
		t.Fatalf("unexpected model or object calls: %d / %d", modelCalls.Load(), objectCalls.Load())
	}
	writeUploadAgentMessage(t, conn, protocol.TypeSessionHistory, "history", protocol.SessionHistoryPayload{SessionID: sessionID})
	history := readUploadAgentMessage(t, conn)
	assertIntegrationNoSecrets(t, history)
	if history.Type != protocol.TypeSessionHistoryResult || !bytes.Contains(history.Payload, []byte(integrationAssetID)) || bytes.Contains(history.Payload, []byte(base64.StdEncoding.EncodeToString(original))) {
		t.Fatal("history did not preserve the image reference alone")
	}
}
