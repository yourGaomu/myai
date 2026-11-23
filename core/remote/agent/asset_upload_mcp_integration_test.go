package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	shortlinkssource "myai/core/adapter/documentsource/shortlinks"
	shortlinksfiles "myai/core/adapter/files/shortlinks"
	documentcommand "myai/core/application/knowledge/document/command"
	documentresult "myai/core/application/knowledge/document/result"
	querycommand "myai/core/application/knowledge/query/command"
	queryresult "myai/core/application/knowledge/query/result"
	domainknowledge "myai/core/domain/knowledge"
	"myai/core/mcp"
	"myai/core/port/files"
	"myai/core/remote/protocol"
	"myai/core/tool"
	"myai/core/trust"
)

type ingestAttachmentLookup struct {
	ChatFacade
	lookup files.SessionAssetLookup
}

func (c ingestAttachmentLookup) LookupSessionAsset(ctx context.Context, sessionID, assetID string) (files.Asset, error) {
	return c.lookup(ctx, sessionID, assetID)
}

const integrationAssetID = "ast_mobile_integration"

var integrationAssetCreatedAt = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

// The facade substitutes only document persistence. Its source and file service
// are the production adapters, so ingest really downloads the uploaded AssetID.
type sourceIngestFacade struct {
	KnowledgeFacade
	source   shortlinkssource.Source
	commands chan documentcommand.Ingest
	contents chan domainknowledge.DocumentSourceContent
	mu       sync.Mutex
	document domainknowledge.Document
}

func (f *sourceIngestFacade) IngestDocument(ctx context.Context, command documentcommand.Ingest) (documentresult.Ingest, error) {
	content, err := f.source.Read(ctx, domainknowledge.DocumentSourceRequest{AssetID: command.AssetID, MaxBytes: files.MaxFileBytes})
	if err != nil {
		return documentresult.Ingest{}, err
	}
	f.commands <- command
	f.contents <- content
	checksum := sha256.Sum256(content.Data)
	document := domainknowledge.Document{
		ID: "document_mobile", KnowledgeBaseID: command.KnowledgeBaseID,
		FileName: content.FileName, ContentType: content.ContentType, ObjectKey: "knowledge/mobile/report.txt",
		ContentHash: hex.EncodeToString(checksum[:]), Version: 1, Status: domainknowledge.DocumentStatusUploaded,
		CreatedAt: integrationAssetCreatedAt, UpdatedAt: integrationAssetCreatedAt,
	}
	if err := document.Validate(); err != nil {
		return documentresult.Ingest{}, err
	}
	f.mu.Lock()
	f.document = document
	f.mu.Unlock()
	return documentresult.Ingest{Document: document}, nil
}

func (f *sourceIngestFacade) ListDocuments(_ context.Context, command querycommand.Documents) (queryresult.Documents, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if command.KnowledgeBaseID != f.document.KnowledgeBaseID {
		return queryresult.Documents{}, fmt.Errorf("unexpected knowledge base")
	}
	return queryresult.Documents{Documents: []domainknowledge.Document{f.document}}, nil
}

// Only the external MCP server and object store are faked. Responses follow the
// modern structuredContent multipart contract, with nonnil part inventories.
type mobileMCPBackend struct {
	t              *testing.T
	mu             sync.Mutex
	name           string
	mediaType      string
	size           int64
	key            string
	parts          map[int][]byte
	calls          map[string]int
	completed      bool
	downloadURL    string
	omittedTool    string
	rejectComplete bool
}

func (b *mobileMCPBackend) dataLocked() []byte {
	var data []byte
	for number := 1; number <= len(b.parts); number++ {
		data = append(data, b.parts[number]...)
	}
	return data
}

func (b *mobileMCPBackend) assetLocked() map[string]any {
	checksum := sha256.Sum256(b.dataLocked())
	return map[string]any{
		"assetId": integrationAssetID, "businessId": "canvas", "fileName": b.name,
		"contentType": b.mediaType, "sizeBytes": b.size,
		"checksumSha256": hex.EncodeToString(checksum[:]), "createdAt": integrationAssetCreatedAt,
	}
}

func (b *mobileMCPBackend) sessionLocked() map[string]any {
	const partSize = 128 * 1024
	parts := make([]int, 0, len(b.parts))
	var received int64
	for number := 1; number <= int((b.size-1)/partSize+1); number++ {
		if data, exists := b.parts[number]; exists {
			parts = append(parts, number)
			received += int64(len(data))
		}
	}
	result := map[string]any{
		"uploadSessionId": "upl_mobile_integration", "idempotencyKey": b.key,
		"status": "uploading", "fileName": b.name, "businessId": "canvas", "path": "agent-uploads",
		"sizeBytes": b.size, "partSizeBytes": partSize, "partCount": (b.size-1)/partSize + 1,
		"receivedParts": parts, "receivedBytes": received, "expiresAt": time.Now().Add(time.Hour),
	}
	if b.completed {
		result["status"], result["asset"] = "completed", b.assetLocked()
	}
	return result
}

func integrationArgument[T any](args map[string]json.RawMessage, key string) T {
	var value T
	_ = json.Unmarshal(args[key], &value)
	return value
}

func (b *mobileMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer mcp-integration-secret" || r.Header.Get("X-API-Key") != "integration-api-secret" {
		b.t.Error("managed MCP request lost configured authentication")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var request struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params struct {
			Name      string                     `json:"name"`
			Arguments map[string]json.RawMessage `json:"arguments"`
		} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		b.t.Error(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var result any
	switch request.Method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "integration-mcp-session-secret")
		result = map[string]any{"protocolVersion": "2025-03-26"}
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
		return
	case "tools/list":
		var infos []mcp.ToolInfo
		for _, name := range []string{
			"begin_asset_upload", "upload_asset_part", "get_asset_upload_status", "complete_asset_upload",
			"get_asset_info", "get_asset_download_url", "create_asset_shortlink", "create_url_shortlink", "upload_file_and_share",
		} {
			if name != b.omittedTool {
				infos = append(infos, mcp.ToolInfo{Name: name})
			}
		}
		result = map[string]any{"tools": infos}
	case "tools/call":
		if r.Header.Get("Mcp-Session-Id") != "integration-mcp-session-secret" {
			b.t.Error("MCP tool call lost managed session")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		name, args := request.Params.Name, request.Params.Arguments
		b.calls[name]++
		var structured any
		var rejected bool
		switch name {
		case "begin_asset_upload":
			b.name = integrationArgument[string](args, "fileName")
			b.mediaType = integrationArgument[string](args, "contentType")
			b.size = integrationArgument[int64](args, "sizeBytes")
			b.key = integrationArgument[string](args, "idempotencyKey")
			structured = b.sessionLocked()
		case "upload_asset_part":
			if integrationArgument[string](args, "uploadSessionId") != "upl_mobile_integration" {
				b.t.Error("part uploaded to wrong MCP session")
			}
			data, err := base64.StdEncoding.DecodeString(integrationArgument[string](args, "contentBase64"))
			if err != nil {
				b.t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			b.parts[integrationArgument[int](args, "partNumber")] = data
			structured = b.sessionLocked()
		case "get_asset_upload_status":
			structured = b.sessionLocked()
		case "complete_asset_upload":
			if b.rejectComplete {
				rejected = true
				structured = map[string]any{"error": "X-Amz-Signature=integration-signed-secret"}
				break
			}
			b.completed = true
			structured = b.sessionLocked()
		case "get_asset_download_url":
			if !b.completed || integrationArgument[string](args, "assetId") != integrationAssetID {
				b.t.Error("knowledge ingest did not request the uploaded AssetID")
			}
			structured = map[string]any{"asset": b.assetLocked(), "downloadUrl": b.downloadURL, "expiresAt": time.Now().Add(time.Hour)}
		case "get_asset_info":
			structured = b.assetLocked()
		default:
			b.t.Errorf("unexpected legacy or public sharing tool call: %s", name)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Any accidental use of untrusted MCP text exposes these sentinels and
		// fails the outward-message assertions below.
		result = map[string]any{"isError": rejected, "structuredContent": structured, "content": []any{map[string]any{
			"type": "text", "text": "mcp-integration-secret integration-api-secret X-Amz-Signature=integration-signed-secret",
		}}}
	default:
		b.t.Errorf("unexpected MCP method: %s", request.Method)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}

func assertIntegrationNoSecrets(t *testing.T, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"mcp-integration-secret", "integration-api-secret", "integration-mcp-session-secret",
		"integration-signed-secret", "integration-signed-credential", "integration-signed-token",
		"X-Amz-", "downloadUrl", "download_url", "paired-token",
	} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("private credential or signed download transport escaped in %T", value)
		}
	}
}

func readIntegrationUploadResult(t *testing.T, conn *websocket.Conn, request, operation string) protocol.AssetUploadResultPayload {
	t.Helper()
	message := readUploadAgentMessage(t, conn)
	assertIntegrationNoSecrets(t, message)
	if message.Type != protocol.TypeAssetUploadResult || message.RequestID != request || message.SessionID != "session" {
		t.Fatalf("unexpected upload result: type=%s request=%s", message.Type, message.RequestID)
	}
	result, err := protocol.DecodePayload[protocol.AssetUploadResultPayload](message)
	if err != nil || result.Operation != operation || result.ChunkSize != 262144 || result.MaxFileBytes != files.MaxFileBytes {
		t.Fatalf("invalid upload result contract: %#v %v", result, err)
	}
	return result
}

func TestAgentMobileUploadMCPAndKnowledgeAssetIDIntegration(t *testing.T) {
	content := bytes.Repeat([]byte("mobile knowledge report\n"), 14000)
	backend := &mobileMCPBackend{t: t, parts: make(map[int][]byte), calls: make(map[string]int)}
	var objectCalls atomic.Int32
	object := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		objectCalls.Add(1)
		if r.URL.Query().Get("X-Amz-Signature") != "integration-signed-secret" {
			t.Error("source did not use the MCP-issued signed object URL")
		}
		for _, header := range []string{"Authorization", "X-API-Key", "Mcp-Session-Id", "Mcp-Protocol-Version", "Cookie", "Referer"} {
			if r.Header.Get(header) != "" {
				t.Errorf("object download leaked MCP/client header %s", header)
			}
		}
		backend.mu.Lock()
		data := backend.dataLocked()
		backend.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		_, _ = w.Write(data)
	}))
	t.Cleanup(object.Close)
	backend.downloadURL = object.URL + "/object?X-Amz-Signature=integration-signed-secret&X-Amz-Credential=integration-signed-credential&X-Amz-Security-Token=integration-signed-token"
	mcpServer := httptest.NewServer(backend)
	t.Cleanup(mcpServer.Close)
	registry := tool.NewRegisterTools()
	manager := mcp.NewManager(mcp.Config{Servers: []mcp.ServerConfig{{
		Name: "short_links", URL: mcpServer.URL + "/api/mcp", Required: true, TimeoutSeconds: 5,
		Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(),
		Headers: map[string]string{"Authorization": "Bearer mcp-integration-secret", "X-API-Key": "integration-api-secret"},
	}}})
	if err := manager.RegisterAll(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	fileService := shortlinksfiles.New(func() *mcp.Manager { return manager })
	knowledge := &sourceIngestFacade{
		source:   shortlinkssource.Source{Client: fileService},
		commands: make(chan documentcommand.Ingest, 1), contents: make(chan domainknowledge.DocumentSourceContent, 1),
	}
	type recordedAsset struct {
		session, request string
		asset            files.Asset
	}
	recorded := make(chan recordedAsset, 2)
	var attached atomic.Pointer[files.Asset]
	lookup := ingestAttachmentLookup{lookup: func(_ context.Context, sessionID, assetID string) (files.Asset, error) {
		pending := attached.Load()
		if pending != nil && sessionID == "session" && assetID == pending.AssetID {
			return *pending, nil
		}
		return files.Asset{}, files.ErrSessionAssetUnavailable
	}}
	a := New(Config{UserID: "user", DeviceID: "device"}, lookup, nil, nil, knowledge, nil, nil, nil, nil, nil, nil,
		WithFileService(fileService), WithFileUploadAdmission(testUploadAdmission), WithFileAssetRecorder(func(_ context.Context, session, request string, epoch uint64, asset files.Asset) error {
			if epoch != 7 {
				t.Error("recorder lost begin admission epoch")
			}
			attached.Store(&asset)
			recorded <- recordedAsset{session, request, asset}
			return nil
		}))
	conn := openUploadAgent(t, a)
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadBegin, "mobile-begin", protocol.AssetUploadBeginPayload{
		FileName: "report.txt", ContentType: "text/plain", Size: int64(len(content)), IdempotencyKey: "mobile-integration-key",
	})
	begun := readIntegrationUploadResult(t, conn, "mobile-begin", "begin")
	// This is the real readLoop boundary used after Relay authentication. A
	// different paired client cannot take over the pending private upload.
	wrongOwner, err := protocol.NewMessage(protocol.TypeAssetUploadStatus, "wrong-owner", "user", "device", "session", protocol.AssetUploadStatusPayload{UploadID: begun.UploadID})
	if err != nil {
		t.Fatal(err)
	}
	wrongOwner.ClientToken = "different-paired-client"
	if err := conn.WriteJSON(wrongOwner); err != nil {
		t.Fatal(err)
	}
	denied := readUploadAgentMessage(t, conn)
	assertIntegrationNoSecrets(t, denied)
	if denied.Type != protocol.TypeError || denied.RequestID != "wrong-owner" {
		t.Fatal("readLoop accepted a different upload owner")
	}
	for offset := 0; offset < len(content); offset += protocol.AssetUploadChunkSize {
		end := min(offset+protocol.AssetUploadChunkSize, len(content))
		request := fmt.Sprintf("mobile-chunk-%d", offset)
		writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadChunk, request, protocol.AssetUploadChunkPayload{
			UploadID: begun.UploadID, Offset: int64(offset), Data: base64.StdEncoding.EncodeToString(content[offset:end]),
		})
		chunk := readIntegrationUploadResult(t, conn, request, "chunk")
		if chunk.Offset != int64(end) {
			t.Fatal("mobile chunk acknowledgement lost progress")
		}
	}
	var uploadedAssetID string
	for _, request := range []string{"mobile-complete", "mobile-complete-retry"} {
		writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadComplete, request, protocol.AssetUploadCompletePayload{UploadID: begun.UploadID})
		completed := readIntegrationUploadResult(t, conn, request, "complete")
		if completed.Asset == nil || completed.Asset.AssetID != integrationAssetID || completed.Asset.BusinessID != "canvas" ||
			completed.Asset.Size != int64(len(content)) || completed.Asset.ShortURL != "" || completed.Asset.Code != "" {
			t.Fatalf("invalid private upload asset receipt: %#v", completed.Asset)
		}
		uploadedAssetID = completed.Asset.AssetID
	}
	select {
	case record := <-recorded:
		assertIntegrationNoSecrets(t, record.asset)
		if record.session != "session" || record.request != "mobile-complete" || record.asset.AssetID != integrationAssetID ||
			!record.asset.CreatedAt.Equal(integrationAssetCreatedAt) {
			t.Fatal("recorder lost stable request, asset identity or backend creation time")
		}
	default:
		t.Fatal("upload completed without recording its session asset")
	}
	select {
	case <-recorded:
		t.Fatal("complete retry duplicated the recorder callback")
	default:
	}
	for _, deniedCase := range []struct{ request, user, session, asset string }{
		{"ingest-other-session", "user", "other-session", uploadedAssetID},
		{"ingest-unattached", "user", "session", "ast_unattached"},
		{"ingest-missing-session", "user", "", uploadedAssetID},
		{"ingest-missing-pair", "", "session", uploadedAssetID},
	} {
		message, err := protocol.NewMessage(protocol.TypeKnowledgeDocumentIngest, deniedCase.request, deniedCase.user, "device", deniedCase.session, protocol.KnowledgeDocumentIngestPayload{
			KnowledgeBaseID: "kb_mobile", AssetID: deniedCase.asset,
		})
		if err != nil {
			t.Fatal(err)
		}
		message.ClientToken = "paired-client"
		if err := conn.WriteJSON(message); err != nil {
			t.Fatal(err)
		}
		if reply := readUploadAgentMessage(t, conn); reply.Type != protocol.TypeError || reply.RequestID != deniedCase.request {
			t.Fatalf("admission returned %s for %s", reply.Type, deniedCase.request)
		}
	}
	backend.mu.Lock()
	downloadCalls := backend.calls["get_asset_download_url"]
	backend.mu.Unlock()
	if objectCalls.Load() != 0 || downloadCalls != 0 || len(knowledge.commands) != 0 {
		t.Fatal("denied ingest reached the document source")
	}
	writeUploadAgentMessage(t, conn, protocol.TypeKnowledgeDocumentIngest, "mobile-ingest", protocol.KnowledgeDocumentIngestPayload{
		KnowledgeBaseID: "kb_mobile", AssetID: uploadedAssetID,
	})
	ingested := readUploadAgentMessage(t, conn)
	assertIntegrationNoSecrets(t, ingested)
	if ingested.Type != protocol.TypeKnowledgeDocumentMutationResult || ingested.RequestID != "mobile-ingest" {
		t.Fatalf("knowledge ingest failed through mobile protocol: %s", ingested.Type)
	}
	mutation, err := protocol.DecodePayload[protocol.KnowledgeDocumentListResultPayload](ingested)
	if err != nil || len(mutation.Documents) != 1 || mutation.Documents[0].KnowledgeBaseID != "kb_mobile" || mutation.Documents[0].FileName != "report.txt" {
		t.Fatalf("invalid knowledge mutation result: %#v %v", mutation, err)
	}
	select {
	case command := <-knowledge.commands:
		if command.AssetID != uploadedAssetID || command.KnowledgeBaseID != "kb_mobile" {
			t.Fatal("knowledge facade did not receive uploaded AssetID")
		}
	default:
		t.Fatal("knowledge command did not reach the facade")
	}
	select {
	case downloaded := <-knowledge.contents:
		assertIntegrationNoSecrets(t, downloaded)
		if !bytes.Equal(downloaded.Data, content) || downloaded.Size != int64(len(content)) || downloaded.ContentType != "text/plain" {
			t.Fatal("real knowledge source did not recover the original uploaded content")
		}
	default:
		t.Fatal("real knowledge source was not used")
	}
	if objectCalls.Load() != 1 {
		t.Fatalf("knowledge source object downloads=%d", objectCalls.Load())
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if !bytes.Equal(backend.dataLocked(), content) || backend.calls["begin_asset_upload"] != 1 || backend.calls["complete_asset_upload"] != 1 ||
		backend.calls["upload_asset_part"] != (len(content)-1)/(128*1024)+1 || backend.calls["get_asset_download_url"] != 1 {
		t.Fatalf("unexpected modern MCP calls: %#v", backend.calls)
	}
	if backend.key == "mobile-integration-key" || !strings.HasPrefix(backend.key, "remote-upload-") {
		t.Fatal("mobile idempotency key was not scoped to its authenticated owner")
	}
	for _, name := range []string{"upload_file_and_share", "create_url_shortlink", "create_asset_shortlink", "upload_file", "download_file"} {
		if backend.calls[name] != 0 {
			t.Fatalf("upload/ingest invoked legacy or public sharing tool %s", name)
		}
	}
}

func readIntegrationUploadError(t *testing.T, conn *websocket.Conn, request string, kind protocol.MessageType, code string) {
	t.Helper()
	message := readUploadAgentMessage(t, conn)
	assertIntegrationNoSecrets(t, message)
	var failure struct {
		Message     string               `json:"message"`
		Code        string               `json:"code"`
		RequestType protocol.MessageType `json:"request_type"`
	}
	if message.Type != protocol.TypeError || message.RequestID != request || message.SessionID != "session" ||
		json.Unmarshal(message.Payload, &failure) != nil || failure.RequestType != kind || failure.Code != code || failure.Message == "" {
		t.Fatalf("unexpected upload error type=%s request=%s session=%s payload=%s", message.Type, message.RequestID, message.SessionID, message.Payload)
	}
}

func integrationMCPConfig(t *testing.T, url string) mcp.Config {
	t.Helper()
	return mcp.Config{Servers: []mcp.ServerConfig{{
		Name: "short_links", URL: url + "/api/mcp", Required: true, TimeoutSeconds: 5,
		Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(),
		Headers: map[string]string{"Authorization": "Bearer mcp-integration-secret", "X-API-Key": "integration-api-secret"},
	}}}
}

func TestAgentMobileUploadMCPReadiness(t *testing.T) {
	for _, tc := range []struct{ name, code string }{
		{"file service unavailable", "upload_service_unavailable"},
		{"manager unavailable", "upload_service_unavailable"},
		{"short_links unconfigured", "upload_service_unavailable"},
		{"missing required tool", "upload_missing_tool"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var manager *mcp.Manager
			var backend *mobileMCPBackend
			switch tc.name {
			case "short_links unconfigured":
				manager = mcp.NewManager(mcp.Config{})
			case "missing required tool":
				backend = &mobileMCPBackend{t: t, calls: make(map[string]int), omittedTool: "upload_asset_part"}
				server := httptest.NewServer(backend)
				t.Cleanup(server.Close)
				manager = mcp.NewManager(integrationMCPConfig(t, server.URL))
			}
			if manager != nil {
				t.Cleanup(func() { _ = manager.Close() })
				if err := manager.RegisterAll(context.Background(), tool.NewRegisterTools()); err != nil {
					t.Fatal(err)
				}
			}
			var fileService files.Service
			if tc.name != "file service unavailable" {
				service := shortlinksfiles.New(func() *mcp.Manager { return manager })
				t.Cleanup(func() { _ = service.Close() })
				fileService = service
			}
			a := New(Config{UserID: "user", DeviceID: "device"}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				WithFileService(fileService), WithFileUploadAdmission(testUploadAdmission))
			conn := openUploadAgent(t, a)
			writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadBegin, "denied-begin", protocol.AssetUploadBeginPayload{
				FileName: "report.txt", Size: 7, IdempotencyKey: "denied-key",
			})
			readIntegrationUploadError(t, conn, "denied-begin", protocol.TypeAssetUploadBegin, tc.code)
			if backend != nil {
				backend.mu.Lock()
				defer backend.mu.Unlock()
				if len(backend.calls) != 0 {
					t.Fatal("preflight dispatched MCP tool calls")
				}
			}
		})
	}
}

func TestAgentMobileUploadMCPRejectionAndRecoveryAfterReload(t *testing.T) {
	ctx := context.Background()
	backend := &mobileMCPBackend{t: t, parts: make(map[int][]byte), calls: make(map[string]int), rejectComplete: true}
	server := httptest.NewServer(backend)
	t.Cleanup(server.Close)
	manager := mcp.NewManager(integrationMCPConfig(t, server.URL))
	t.Cleanup(func() { _ = manager.Close() })
	registry := tool.NewRegisterTools()
	if err := manager.RegisterAll(ctx, registry); err != nil {
		t.Fatal(err)
	}
	service := shortlinksfiles.New(func() *mcp.Manager { return manager })
	t.Cleanup(func() { _ = service.Close() })
	var recorded atomic.Int32
	a := New(Config{UserID: "user", DeviceID: "device"}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		WithFileService(service), WithFileUploadAdmission(testUploadAdmission), WithFileAssetRecorder(func(_ context.Context, session, request string, epoch uint64, asset files.Asset) error {
			if session != "session" || request != "first-complete" || epoch != 7 || asset.AssetID != integrationAssetID {
				t.Error("recorder lost upload identity on recovery")
			}
			recorded.Add(1)
			return nil
		}))
	conn := openUploadAgent(t, a)
	content := []byte("content")
	id := stageAgentUpload(t, conn, content)
	complete := protocol.AssetUploadCompletePayload{UploadID: id}
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadComplete, "first-complete", complete)
	readIntegrationUploadError(t, conn, "first-complete", protocol.TypeAssetUploadComplete, "upload_upstream_failure")
	if recorded.Load() != 0 {
		t.Fatal("rejected upload reached the session recorder")
	}
	if err := manager.Reload(ctx, mcp.Config{}, registry); err != nil {
		t.Fatal(err)
	}
	begin := protocol.AssetUploadBeginPayload{
		FileName: "report.txt", ContentType: "text/plain", Size: int64(len(content)), IdempotencyKey: "key",
	}
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadBegin, "begin-retry", begin)
	retry := readIntegrationUploadResult(t, conn, "begin-retry", "begin")
	if retry.UploadID != id || retry.Offset != int64(len(content)) || retry.Asset != nil {
		t.Fatalf("readiness change lost staged receipt: %#v", retry)
	}
	begin.IdempotencyKey = "new-key"
	writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadBegin, "new-begin", begin)
	readIntegrationUploadError(t, conn, "new-begin", protocol.TypeAssetUploadBegin, "upload_service_unavailable")

	backend.mu.Lock()
	backend.rejectComplete = false
	backend.mu.Unlock()
	for _, request := range []string{"complete-retry", "completed-retry"} {
		writeUploadAgentMessage(t, conn, protocol.TypeAssetUploadComplete, request, complete)
		result := readIntegrationUploadResult(t, conn, request, "complete")
		if result.UploadID != id || result.Asset == nil || result.Asset.AssetID != integrationAssetID {
			t.Fatalf("could not recover existing upload: %#v", result)
		}
	}
	if recorded.Load() != 1 {
		t.Fatal("completion recovery did not record exactly one asset")
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.calls["begin_asset_upload"] != 1 || backend.calls["upload_asset_part"] != 1 ||
		backend.calls["get_asset_upload_status"] != 1 || backend.calls["complete_asset_upload"] != 2 {
		t.Fatalf("recovery did not reuse the original upload: %#v", backend.calls)
	}
}
