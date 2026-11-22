package shortlinks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"myai/core/mcp"
	filesport "myai/core/port/files"
	"myai/core/tool"
	"myai/core/trust"
)

type mockBackend struct {
	mu          sync.Mutex
	size        int64
	partSize    int64
	name        string
	contentType string
	key         string
	parts       map[int][]byte
	completed   bool
	calls       map[string]int
	begins      []string
	onCall      func(string, map[string]json.RawMessage)
	override    func(string, map[string]json.RawMessage) (any, int, bool)
	downloadURL string
}

func newBackend() *mockBackend {
	return &mockBackend{partSize: 4, parts: make(map[int][]byte), calls: make(map[string]int)}
}

func (b *mockBackend) asset() map[string]any {
	data := make([]byte, 0, b.size)
	for part := 1; int64(part) <= (b.size-1)/b.partSize+1; part++ {
		data = append(data, b.parts[part]...)
	}
	checksum := sha256.Sum256(data)
	return map[string]any{"assetId": "ast_test", "businessId": "canvas", "fileName": b.name,
		"sizeBytes": b.size, "contentType": b.contentType, "checksumSha256": hex.EncodeToString(checksum[:]),
		"createdAt": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (b *mockBackend) session() map[string]any {
	status := "uploading"
	parts := make([]int, 0, len(b.parts))
	var received int64
	for number, data := range b.parts {
		parts = append(parts, number)
		received += int64(len(data))
	}
	result := map[string]any{"uploadSessionId": "upl_test", "idempotencyKey": b.key, "status": status,
		"fileName": b.name, "businessId": "canvas", "path": "agent-uploads", "sizeBytes": b.size,
		"partSizeBytes": b.partSize, "partCount": (b.size-1)/b.partSize + 1, "receivedParts": parts,
		"receivedBytes": received, "expiresAt": time.Now().Add(time.Hour)}
	if b.completed {
		result["status"], result["asset"] = "completed", b.asset()
	}
	return result
}

func argument[T any](args map[string]json.RawMessage, key string) T {
	var value T
	_ = json.Unmarshal(args[key], &value)
	return value
}

func (b *mockBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params struct {
			Name      string                     `json:"name"`
			Arguments map[string]json.RawMessage `json:"arguments"`
		} `json:"params"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var result any
	switch request.Method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "session-test")
		result = map[string]any{"protocolVersion": "2025-03-26"}
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
		return
	case "tools/list":
		names := append(append([]string(nil), uploadTools...), "cancel_asset_upload", "get_asset_info", "get_asset_download_url", "create_asset_shortlink", "create_url_shortlink", "upload_file_and_share")
		infos := make([]mcp.ToolInfo, 0, len(names))
		for _, name := range names {
			infos = append(infos, mcp.ToolInfo{Name: name})
		}
		result = map[string]any{"tools": infos}
	case "tools/call":
		if r.Header.Get("Mcp-Session-Id") != "session-test" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		name, args := request.Params.Name, request.Params.Arguments
		if b.onCall != nil {
			b.onCall(name, args)
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.calls[name]++
		if b.override != nil {
			if response, status, handled := b.override(name, args); handled {
				if status != 0 {
					w.WriteHeader(status)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": response})
				return
			}
		}
		var structured any
		switch name {
		case "begin_asset_upload":
			b.size, b.name, b.contentType = argument[int64](args, "sizeBytes"), argument[string](args, "fileName"), argument[string](args, "contentType")
			b.key = argument[string](args, "idempotencyKey")
			b.begins = append(b.begins, b.key)
			structured = b.session()
		case "upload_asset_part":
			data, err := base64.StdEncoding.DecodeString(argument[string](args, "contentBase64"))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			b.parts[argument[int](args, "partNumber")] = data
			structured = b.session()
		case "get_asset_upload_status":
			structured = b.session()
		case "complete_asset_upload":
			b.completed = true
			structured = b.session()
		case "create_asset_shortlink":
			if _, exists := args["ttlSeconds"]; exists {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			structured = map[string]any{"asset": b.asset(), "shortUrl": "https://short.example/r/file", "code": "file", "expiresAt": time.Now().Add(time.Hour), "maxVisits": int64(1)}
		case "get_asset_info":
			structured = b.asset()
		case "get_asset_download_url":
			structured = map[string]any{"asset": b.asset(), "downloadUrl": b.downloadURL, "expiresAt": time.Now().Add(time.Hour)}
		}
		result = map[string]any{"structuredContent": structured, "content": []any{map[string]any{"type": "text", "text": "untrusted text ignored"}}}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}

func startService(t *testing.T, backend *mockBackend) (*Service, *mcp.Manager, *tool.RegisterTools) {
	t.Helper()
	server := httptest.NewServer(backend)
	t.Cleanup(server.Close)
	registry := tool.NewRegisterTools()
	manager := mcp.NewManager(mcp.Config{Servers: []mcp.ServerConfig{{Name: serverName, URL: server.URL + "/api/mcp", Required: true, Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(), TimeoutSeconds: 5,
		Headers: map[string]string{"Authorization": "Bearer secret", "X-API-Key": "api-secret"}}}})
	if err := manager.RegisterAll(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return New(func() *mcp.Manager { return manager }), manager, registry
}

func TestMultipartShareInfoAndDownloadThroughManagedMCP(t *testing.T) {
	backend := newBackend()
	var objectCalls atomic.Int32
	object := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		objectCalls.Add(1)
		for _, header := range []string{"Authorization", "X-API-Key", "Mcp-Session-Id", "Mcp-Protocol-Version", "Cookie", "Referer"} {
			if r.Header.Get(header) != "" {
				t.Errorf("object fetch leaked %s", header)
			}
		}
		w.Header().Set("Content-Length", "10")
		_, _ = io.WriteString(w, "abcdefghij")
	}))
	defer object.Close()
	backend.downloadURL = object.URL + "/object?X-Amz-Signature=never-return-this"
	service, _, registry := startService(t, backend)
	request := filesport.ShareRequest{UploadRequest: filesport.UploadRequest{Reader: strings.NewReader("abcdefghij"), Size: 10, FileName: "test.txt"}, TTLSeconds: 3600, MaxVisits: 1}
	asset, err := service.Share(context.Background(), request)
	if err != nil || asset.AssetID != "ast_test" || asset.ShortURL == "" || asset.Size != 10 {
		t.Fatalf("share = %+v, %v", asset, err)
	}
	if backend.key == "" || backend.calls["upload_asset_part"] != 3 || backend.calls["create_asset_shortlink"] != 1 {
		t.Fatalf("unexpected upload calls: %+v", backend.calls)
	}
	for _, info := range registry.List() {
		if strings.Contains(info.Name(), "upload_asset_part") || strings.Contains(info.Name(), "upload_file_and_share") || strings.Contains(info.Name(), "get_asset_download_url") {
			t.Fatalf("raw file tool exposed to model: %s", info.Name())
		}
	}
	publication, err := registry.GetTool("mcp_short_links_create_asset_shortlink")
	if err != nil || !publication.(interface{ RequiresExplicitApproval() bool }).RequiresExplicitApproval() {
		t.Fatal("publication did not require explicit approval")
	}
	info, err := service.Info(context.Background(), "ast_test")
	if err != nil || info.AssetID != asset.AssetID || info.ShortURL != "" {
		t.Fatalf("info = %+v, %v", info, err)
	}
	download, err := service.Download(context.Background(), filesport.DownloadRequest{AssetID: "ast_test", MaxBytes: 4})
	if err != nil || string(download.Data) != "abcd" || !download.Truncated || download.Asset.ShortURL != "" {
		t.Fatalf("download = %+v, %v", download, err)
	}
	download, err = service.Download(context.Background(), filesport.DownloadRequest{AssetID: "ast_test"})
	if err != nil || string(download.Data) != "abcdefghij" || download.Truncated || objectCalls.Load() != 2 {
		t.Fatalf("full download = %+v, %v", download, err)
	}
	output, err := publication.Call(context.Background(), json.RawMessage(`{"assetId":"ast_test"}`))
	if err != nil || output.SharedAsset == nil || output.SharedAsset.AssetID != "ast_test" || !json.Valid([]byte(output.Content)) {
		t.Fatalf("normalized publication = %+v, %v", output, err)
	}
}

func TestUploadBoundaryAndInputValidation(t *testing.T) {
	backend := newBackend()
	backend.partSize = maxPartBytes
	service, _, _ := startService(t, backend)
	for _, size := range []int64{0, -1, filesport.MaxFileBytes + 1} {
		_, err := service.Upload(context.Background(), filesport.UploadRequest{Reader: strings.NewReader("x"), Size: size, FileName: "test.txt"})
		if err == nil {
			t.Fatalf("accepted size %d", size)
		}
	}
	if _, err := service.Upload(context.Background(), filesport.UploadRequest{Reader: strings.NewReader("ab"), Size: 1, FileName: "test.txt"}); err == nil {
		t.Fatal("accepted oversized reader")
	}
	if _, err := service.Upload(context.Background(), filesport.UploadRequest{Reader: strings.NewReader("a"), Size: 2, FileName: "test.txt"}); err == nil {
		t.Fatal("accepted short reader")
	}
	data := bytes.Repeat([]byte{42}, int(filesport.MaxFileBytes))
	asset, err := service.Upload(context.Background(), filesport.UploadRequest{Reader: bytes.NewReader(data), Size: int64(len(data)), FileName: "boundary.bin"})
	if err != nil || asset.Size != filesport.MaxFileBytes {
		t.Fatalf("50MiB boundary = %+v, %v", asset, err)
	}
	if int64(len(backend.parts[1])) != maxPartBytes || int64(len(backend.parts[2])) != filesport.MaxFileBytes-maxPartBytes || backend.calls["create_asset_shortlink"] != 0 {
		t.Fatal("multipart boundary or nonpublishing upload mismatch")
	}
}

func TestMultipartLeaseSurvivesReloadDuringTransfer(t *testing.T) {
	backend := newBackend()
	entered, resume := make(chan struct{}), make(chan struct{})
	backend.onCall = func(name string, args map[string]json.RawMessage) {
		if name == "upload_asset_part" && argument[int](args, "partNumber") == 1 {
			close(entered)
			<-resume
		}
	}
	service, manager, registry := startService(t, backend)
	oldLease, err := manager.AcquireServer(context.Background(), serverName)
	if err != nil {
		t.Fatal(err)
	}
	oldOrigin := oldLease.Endpoint()
	_ = oldLease.Close()
	finished := make(chan error, 1)
	go func() {
		_, err := service.Upload(context.Background(), filesport.UploadRequest{Reader: strings.NewReader("abcdefghij"), Size: 10, FileName: "test.txt"})
		finished <- err
	}()
	<-entered
	candidate := newBackend()
	candidateServer := httptest.NewServer(candidate)
	defer candidateServer.Close()
	if err := manager.Reload(context.Background(), mcp.Config{Servers: []mcp.ServerConfig{{Name: serverName, URL: candidateServer.URL, Required: true, Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir()}}}, registry); err != nil {
		close(resume)
		t.Fatal(err)
	}
	newLease, err := manager.AcquireServer(context.Background(), serverName)
	if err != nil || newLease.Endpoint() == oldOrigin {
		close(resume)
		t.Fatalf("new runtime = %v", err)
	}
	_ = newLease.Close()
	close(resume)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if backend.calls["complete_asset_upload"] != 1 || len(candidate.calls) != 0 {
		t.Fatalf("transfer changed runtimes: old=%v new=%v", backend.calls, candidate.calls)
	}
}

func TestCancellationAndUnavailableProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := New(func() *mcp.Manager { return nil })
	_, err := service.Upload(ctx, filesport.UploadRequest{Reader: strings.NewReader("a"), Size: 1, FileName: "test.txt"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	if _, err := service.Info(context.Background(), "ast_test"); err == nil {
		t.Fatal("nil provider manager accepted")
	}
	backend := newBackend()
	entered := make(chan struct{})
	backend.onCall = func(name string, _ map[string]json.RawMessage) {
		if name == "upload_asset_part" {
			close(entered)
		}
	}
	service, _, _ = startService(t, backend)
	ctx, cancel = context.WithCancel(context.Background())
	backend.override = func(name string, _ map[string]json.RawMessage) (any, int, bool) {
		if name == "upload_asset_part" {
			cancel()
			return nil, http.StatusServiceUnavailable, true
		}
		return nil, 0, false
	}
	_, err = service.Upload(ctx, filesport.UploadRequest{Reader: strings.NewReader("abcdefghij"), Size: 10, FileName: "test.txt"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("in-flight cancel = %v", err)
	}
	select {
	case <-entered:
	default:
		t.Fatal("part request did not start")
	}
}

func TestUncertainPartAndCompletionUseStatusWithoutDuplicateCreate(t *testing.T) {
	backend := newBackend()
	partLost, completeLost := true, true
	backend.override = func(name string, args map[string]json.RawMessage) (any, int, bool) {
		if name == "upload_asset_part" && partLost {
			partLost = false
			data, _ := base64.StdEncoding.DecodeString(argument[string](args, "contentBase64"))
			backend.parts[argument[int](args, "partNumber")] = data
			return nil, http.StatusServiceUnavailable, true
		}
		if name == "complete_asset_upload" && completeLost {
			completeLost, backend.completed = false, true
			return nil, http.StatusServiceUnavailable, true
		}
		return nil, 0, false
	}
	service, _, _ := startService(t, backend)
	asset, err := service.Upload(context.Background(), filesport.UploadRequest{Reader: strings.NewReader("abcdefghij"), Size: 10, FileName: "test.txt"})
	if err != nil || asset.AssetID != "ast_test" {
		t.Fatalf("recovered = %+v, %v", asset, err)
	}
	if backend.calls["begin_asset_upload"] != 1 || backend.calls["upload_asset_part"] != 3 || backend.calls["complete_asset_upload"] != 1 || backend.calls["get_asset_upload_status"] != 2 {
		t.Fatalf("recovery calls = %v", backend.calls)
	}
}

func TestShareErrorRetainsAssetAndRedactsBackend(t *testing.T) {
	backend := newBackend()
	backend.override = func(name string, _ map[string]json.RawMessage) (any, int, bool) {
		if name == "create_asset_shortlink" {
			return map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": "https://signed.invalid/object?secret=never-leak api-secret"}}}, 0, true
		}
		return nil, 0, false
	}
	service, _, _ := startService(t, backend)
	asset, err := service.Share(context.Background(), filesport.ShareRequest{UploadRequest: filesport.UploadRequest{Reader: strings.NewReader("abc"), Size: 3, FileName: "test.txt"}})
	if err == nil || asset.AssetID != "ast_test" || !strings.Contains(err.Error(), "ast_test") || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "https:") {
		t.Fatalf("share failure = %+v, %v", asset, err)
	}
	if backend.calls["create_asset_shortlink"] != 1 {
		t.Fatal("publication was retried")
	}
}

func TestMalformedIdentityAndPartSizeFailClosed(t *testing.T) {
	for _, mode := range []string{"missing-structured", "wrong-id", "wrong-size", "oversized-part", "is-error"} {
		t.Run(mode, func(t *testing.T) {
			backend := newBackend()
			backend.override = func(name string, args map[string]json.RawMessage) (any, int, bool) {
				if name != "begin_asset_upload" {
					return nil, 0, false
				}
				backend.size, backend.name, backend.contentType, backend.key = 3, "test.txt", "text/plain", argument[string](args, "idempotencyKey")
				data := backend.session()
				switch mode {
				case "missing-structured":
					return map[string]any{"content": []any{map[string]any{"type": "text", "text": "{}"}}}, 0, true
				case "is-error":
					return map[string]any{"isError": true, "structuredContent": data}, 0, true
				case "wrong-id":
					data["uploadSessionId"] = "https://secret.invalid/"
				case "wrong-size":
					data["sizeBytes"] = 4
				case "oversized-part":
					data["partSizeBytes"] = maxPartBytes + 1
				}
				return map[string]any{"structuredContent": data}, 0, true
			}
			service, _, _ := startService(t, backend)
			_, err := service.Upload(context.Background(), filesport.UploadRequest{Reader: strings.NewReader("abc"), Size: 3, FileName: "test.txt"})
			if err == nil || strings.Contains(err.Error(), "secret") || backend.calls["upload_asset_part"] != 0 || backend.calls["begin_asset_upload"] != 1 {
				t.Fatalf("mode %s: %v, calls %v", mode, err, backend.calls)
			}
		})
	}
}

func TestDownloadRejectsUnsafeAddressesExpiredURLsAndSizeMismatch(t *testing.T) {
	for _, mode := range []string{"private-other-host", "metadata", "userinfo", "expired", "wrong-size", "wrong-id", "redirect-loop"} {
		t.Run(mode, func(t *testing.T) {
			backend := newBackend()
			backend.size, backend.name, backend.contentType = 3, "test.txt", "text/plain"
			backend.parts[1] = []byte("abc")
			object := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "redirect-loop" {
					http.Redirect(w, r, "/loop?signature=secret", http.StatusFound)
					return
				}
				_, _ = io.WriteString(w, "abcd")
			}))
			defer object.Close()
			backend.downloadURL = object.URL + "/file?signature=secret"
			backend.override = func(name string, _ map[string]json.RawMessage) (any, int, bool) {
				if name != "get_asset_download_url" {
					return nil, 0, false
				}
				asset, expiry, raw := backend.asset(), time.Now().Add(time.Hour), backend.downloadURL
				switch mode {
				case "private-other-host":
					raw = "https://127.0.0.2/object?secret=yes"
				case "metadata":
					raw = "https://169.254.169.254/latest?secret=yes"
				case "userinfo":
					raw = "https://user:secret@public.example/object"
				case "expired":
					expiry = time.Now().Add(-time.Minute)
				case "wrong-id":
					asset["assetId"] = "ast_wrong"
				}
				return map[string]any{"structuredContent": map[string]any{"asset": asset, "downloadUrl": raw, "expiresAt": expiry}}, 0, true
			}
			service, _, _ := startService(t, backend)
			_, err := service.Download(context.Background(), filesport.DownloadRequest{AssetID: "ast_test"})
			if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "signature") {
				t.Fatalf("%s: %v", mode, err)
			}
		})
	}
}

func TestBeginUncertaintyReusesSameIdempotencyKey(t *testing.T) {
	backend := newBackend()
	var firstKey string
	backend.override = func(name string, args map[string]json.RawMessage) (any, int, bool) {
		if name == "begin_asset_upload" && firstKey == "" {
			firstKey = argument[string](args, "idempotencyKey")
			return nil, http.StatusServiceUnavailable, true
		}
		return nil, 0, false
	}
	service, _, _ := startService(t, backend)
	_, err := service.Upload(context.Background(), filesport.UploadRequest{Reader: strings.NewReader("abc"), Size: 3, FileName: "test.txt"})
	if err != nil || firstKey == "" || backend.key != firstKey || backend.calls["begin_asset_upload"] != 2 {
		t.Fatalf("idempotent begin = %v, first=%s final=%s calls=%v", err, firstKey, backend.key, backend.calls)
	}
}

func ExampleNew() {
	var manager *mcp.Manager
	service := New(func() *mcp.Manager { return manager })
	fmt.Println(service != nil)
	// Output: true
}
