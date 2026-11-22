package shortlinks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"myai/core/mcp"
	filesport "myai/core/port/files"
	"myai/core/trust"
)

func explicitUpload(key, content string) filesport.UploadRequest {
	return filesport.UploadRequest{Reader: strings.NewReader(content), FileName: "test.txt", Size: int64(len(content)), IdempotencyKey: key}
}

func TestCommittedCompletionLostStatusResumesOriginalSessionAfterReload(t *testing.T) {
	backend := newBackend()
	statusDown := true
	backend.override = func(name string, _ map[string]json.RawMessage) (any, int, bool) {
		if name == "complete_asset_upload" {
			backend.completed = true // committed, but acknowledgement was lost
			return nil, http.StatusServiceUnavailable, true
		}
		if name == "get_asset_upload_status" && statusDown {
			return nil, http.StatusServiceUnavailable, true
		}
		return nil, 0, false
	}
	service, manager, registry := startService(t, backend)
	_, err := service.Upload(context.Background(), explicitUpload("stable-receiver-key", "abcdefghij"))
	if err == nil || backend.calls["complete_asset_upload"] != 1 {
		t.Fatalf("expected uncertain completion: %v, calls=%v", err, backend.calls)
	}
	// An elapsed cached deadline must not erase a possibly committed completion.
	service.receipts["stable-receiver-key"].session.ExpiresAt = time.Now().Add(-uploadReceiptTTL - time.Minute)
	candidate := newBackend()
	candidateServer := httptest.NewServer(candidate)
	defer candidateServer.Close()
	if err := manager.Reload(context.Background(), mcp.Config{Servers: []mcp.ServerConfig{{Name: serverName, URL: candidateServer.URL, Required: true, Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir()}}}, registry); err != nil {
		t.Fatal(err)
	}
	backend.mu.Lock()
	statusDown = false
	backend.mu.Unlock()
	asset, err := service.Upload(context.Background(), explicitUpload("stable-receiver-key", "abcdefghij"))
	if err != nil || asset.AssetID != "ast_test" {
		t.Fatalf("resume = %+v, %v", asset, err)
	}
	if backend.calls["begin_asset_upload"] != 1 || backend.calls["complete_asset_upload"] != 1 || backend.calls["upload_asset_part"] != 3 || len(candidate.calls) != 0 {
		t.Fatalf("duplicate creation or changed runtime: old=%v new=%v", backend.calls, candidate.calls)
	}
	asset, err = service.Upload(context.Background(), explicitUpload("stable-receiver-key", "abcdefghij"))
	if err != nil || asset.AssetID != "ast_test" || backend.calls["get_asset_upload_status"] != 2 || len(candidate.calls) != 0 {
		t.Fatalf("completed receipt was not reused: %+v %v", asset, err)
	}
}

func TestReceiptRejectsDifferentContentAndSerializesSameKey(t *testing.T) {
	backend := newBackend()
	service, _, _ := startService(t, backend)
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Upload(context.Background(), explicitUpload("same-key", "abcdefghij"))
			failures <- err
		}()
	}
	wg.Wait()
	for range 2 {
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
	}
	if backend.calls["begin_asset_upload"] != 1 || backend.calls["complete_asset_upload"] != 1 {
		t.Fatalf("concurrent key duplicated side effects: %v", backend.calls)
	}
	if _, err := service.Upload(context.Background(), explicitUpload("same-key", "1234567890")); err == nil || backend.calls["begin_asset_upload"] != 1 {
		t.Fatal("idempotency key accepted different content")
	}
}

func TestPartResponseLostAndStatusUnavailableResumesBeforeSending(t *testing.T) {
	backend := newBackend()
	unavailable := true
	backend.override = func(name string, args map[string]json.RawMessage) (any, int, bool) {
		if name == "upload_asset_part" && unavailable {
			data, _ := base64.StdEncoding.DecodeString(argument[string](args, "contentBase64"))
			backend.parts[argument[int](args, "partNumber")] = data
			return nil, http.StatusServiceUnavailable, true
		}
		if name == "get_asset_upload_status" && unavailable {
			return nil, http.StatusServiceUnavailable, true
		}
		return nil, 0, false
	}
	service, _, _ := startService(t, backend)
	if _, err := service.Upload(context.Background(), explicitUpload("part-recovery", "abcdefghij")); err == nil {
		t.Fatal("expected unknown part outcome")
	}
	backend.mu.Lock()
	unavailable = false
	backend.mu.Unlock()
	if _, err := service.Upload(context.Background(), explicitUpload("part-recovery", "abcdefghij")); err != nil {
		t.Fatal(err)
	}
	if backend.calls["begin_asset_upload"] != 1 || backend.calls["upload_asset_part"] != 3 || backend.calls["get_asset_upload_status"] != 2 {
		t.Fatalf("resumed with duplicate upload: %v", backend.calls)
	}
}

func TestExplicitRetryResumesIdempotentCompletionOfSameSession(t *testing.T) {
	backend := newBackend()
	failComplete := true
	backend.override = func(name string, _ map[string]json.RawMessage) (any, int, bool) {
		if name == "complete_asset_upload" && failComplete {
			failComplete = false
			return map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": "temporary storage error"}}}, 0, true
		}
		return nil, 0, false
	}
	service, _, _ := startService(t, backend)
	if _, err := service.Upload(context.Background(), explicitUpload("complete-retry", "abcdefghij")); err == nil {
		t.Fatal("expected temporary completion error")
	}
	asset, err := service.Upload(context.Background(), explicitUpload("complete-retry", "abcdefghij"))
	if err != nil || asset.AssetID != "ast_test" || backend.calls["begin_asset_upload"] != 1 || backend.calls["get_asset_upload_status"] != 1 || backend.calls["complete_asset_upload"] != 2 {
		t.Fatalf("same-session complete retry: %+v %v calls=%v", asset, err, backend.calls)
	}
}

func TestPreBeginUnavailableManagerDoesNotExhaustReceipts(t *testing.T) {
	var manager *mcp.Manager
	service := New(func() *mcp.Manager { return manager })
	for index := range maxUploadReceipts + 5 {
		if _, err := service.Upload(context.Background(), explicitUpload(fmt.Sprintf("unavailable-%d", index), "abc")); err == nil {
			t.Fatal("missing manager accepted")
		}
	}
	if len(service.receipts) != 0 {
		t.Fatalf("pre-begin failures leaked %d receipts", len(service.receipts))
	}
	backend := newBackend()
	_, manager, _ = startService(t, backend)
	if _, err := service.Upload(context.Background(), explicitUpload("restored", "abc")); err != nil {
		t.Fatal(err)
	}
	_ = service.Close()
}

func TestCloseReleasesRetainedLeaseAndRejectsFutureWork(t *testing.T) {
	backend := newBackend()
	backend.override = func(name string, _ map[string]json.RawMessage) (any, int, bool) {
		if name == "upload_asset_part" || name == "get_asset_upload_status" {
			return nil, http.StatusServiceUnavailable, true
		}
		return nil, 0, false
	}
	service, manager, registry := startService(t, backend)
	if _, err := service.Upload(context.Background(), explicitUpload("close-pending", "abc")); err == nil {
		t.Fatal("expected pending transfer")
	}
	receipt := service.receipts["close-pending"]
	lease := receipt.lease
	candidate := newBackend()
	candidateServer := httptest.NewServer(candidate)
	defer candidateServer.Close()
	if err := manager.Reload(context.Background(), mcp.Config{Servers: []mcp.ServerConfig{{Name: serverName, URL: candidateServer.URL, Required: true, Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir()}}}, registry); err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := lease.CallTool(context.Background(), "get_asset_upload_status", nil); err == nil {
		t.Fatal("shutdown retained old MCP lease")
	}
	if _, err := service.Upload(context.Background(), explicitUpload("after-close", "abc")); err == nil {
		t.Fatal("closed service accepted upload")
	}
	if len(service.receipts) != 0 {
		t.Fatal("shutdown retained receipts")
	}
	_ = service.Close()
}

func TestTerminalCancellationReleasesLeaseAndKeepsKeyFailClosed(t *testing.T) {
	backend := newBackend()
	backend.override = func(name string, _ map[string]json.RawMessage) (any, int, bool) {
		if name == "upload_asset_part" {
			return nil, http.StatusServiceUnavailable, true
		}
		if name == "get_asset_upload_status" {
			data := backend.session()
			data["status"] = "cancelled"
			return map[string]any{"structuredContent": data}, 0, true
		}
		return nil, 0, false
	}
	service, _, _ := startService(t, backend)
	if _, err := service.Upload(context.Background(), explicitUpload("cancelled-key", "abc")); err == nil {
		t.Fatal("cancelled transfer accepted")
	}
	// The cancellation was observed while resolving a failed part; retry uses
	// the same session once to record its terminal state.
	_, _ = service.Upload(context.Background(), explicitUpload("cancelled-key", "abc"))
	receipt := service.receipts["cancelled-key"]
	if !receipt.terminalError || receipt.lease != nil {
		t.Fatal("terminal session retained its lease")
	}
	if _, err := service.Upload(context.Background(), explicitUpload("cancelled-key", "abc")); err == nil || backend.calls["begin_asset_upload"] != 1 {
		t.Fatal("cancelled key recreated an upload")
	}
}

func TestConfirmedBeginRejectionReclaimsCapacity(t *testing.T) {
	backend := newBackend()
	backend.override = func(name string, _ map[string]json.RawMessage) (any, int, bool) {
		if name == "begin_asset_upload" {
			return map[string]any{"isError": true, "structuredContent": map[string]string{"uploadOutcome": "not_created"}}, 0, true
		}
		return nil, 0, false
	}
	service, _, _ := startService(t, backend)
	for index := range maxUploadReceipts + 1 {
		if _, err := service.Upload(context.Background(), explicitUpload(fmt.Sprintf("rejected-%d", index), "abc")); err == nil {
			t.Fatal("backend rejection was acknowledged")
		}
	}
	if len(service.receipts) != 0 || backend.calls["begin_asset_upload"] != maxUploadReceipts+1 {
		t.Fatalf("confirmed failures exhausted recovery capacity: %v", backend.calls)
	}
}

func TestMCPFileCancellationStaysOnOriginalLease(t *testing.T) {
	backend := newBackend()
	cancelAttempts := 0
	backend.override = func(name string, args map[string]json.RawMessage) (any, int, bool) {
		switch name {
		case "upload_asset_part":
			return map[string]any{"isError": true}, 0, true
		case "cancel_asset_upload":
			if argument[string](args, "uploadSessionId") != "upl_test" {
				t.Error("cancel did not use the original session")
			}
			cancelAttempts++
			if cancelAttempts == 1 {
				return map[string]any{"isError": true}, 0, true
			}
			result := backend.session()
			result["status"] = "cancelled"
			return map[string]any{"structuredContent": result}, 0, true
		}
		return nil, 0, false
	}
	service, manager, registry := startService(t, backend)
	ctx := context.Background()
	if _, err := service.Upload(ctx, explicitUpload("scoped-key", "abc")); err == nil {
		t.Fatal("expected partial upload")
	}
	candidate := newBackend()
	server := httptest.NewServer(candidate)
	defer server.Close()
	if err := manager.Reload(ctx, mcp.Config{Servers: []mcp.ServerConfig{{Name: serverName, URL: server.URL, Required: true, Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir()}}}, registry); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CancelUpload(ctx, "scoped-key"); err == nil {
		t.Fatal("cleanup failure was reported as success")
	}
	if _, err := service.Upload(ctx, explicitUpload("scoped-key", "abc")); err == nil {
		t.Fatal("pending cancellation allowed upload to resume")
	}
	if asset, err := service.CancelUpload(ctx, "scoped-key"); err != nil || asset.AssetID != "" {
		t.Fatalf("cancel retry = %+v, %v", asset, err)
	}
	if receipt := service.receipts["scoped-key"]; receipt.lease != nil || !receipt.terminalError || cancelAttempts != 2 || len(candidate.calls) != 0 {
		t.Fatal("cancellation did not release original lease or reached the replaced backend")
	}
}

func TestExpiredStatusReclaimsLease(t *testing.T) {
	backend := newBackend()
	backend.override = func(name string, _ map[string]json.RawMessage) (any, int, bool) {
		if name == "upload_asset_part" {
			return nil, http.StatusServiceUnavailable, true
		}
		if name == "get_asset_upload_status" {
			result := backend.session()
			result["status"], result["expiresAt"] = "expired", time.Now().Add(-time.Minute)
			return map[string]any{"structuredContent": result}, 0, true
		}
		return nil, 0, false
	}
	service, _, _ := startService(t, backend)
	if _, err := service.Upload(context.Background(), explicitUpload("expired-key", "abc")); err == nil {
		t.Fatal("expired session was acknowledged")
	}
	receipt := service.receipts["expired-key"]
	if receipt == nil || receipt.lease != nil || !receipt.terminalError || receipt.finishedAt.IsZero() {
		t.Fatal("confirmed expiry did not release and age out the receipt")
	}
}

func TestTerminalReceiptsExpireWithoutEvictingUncertainTransfers(t *testing.T) {
	service := New(nil)
	for index := range maxUploadReceipts {
		request := explicitUpload(fmt.Sprintf("terminal-%d", index), "abc")
		receipt, done, err := service.receipt(context.Background(), request, []byte("abc"))
		if err != nil {
			t.Fatal(err)
		}
		receipt.started, receipt.terminalError = true, true
		done()
	}
	if _, _, err := service.receipt(context.Background(), explicitUpload("new", "abc"), []byte("abc")); err == nil {
		t.Fatal("terminal tombstones were discarded before their retention period")
	}
	service.mu.Lock()
	for _, receipt := range service.receipts {
		receipt.finishedAt = time.Now().Add(-uploadReceiptTTL - time.Second)
	}
	service.mu.Unlock()
	receipt, done, err := service.receipt(context.Background(), explicitUpload("new", "abc"), []byte("abc"))
	if err != nil {
		t.Fatalf("confirmed terminal sessions permanently exhausted the recovery table: %v", err)
	}
	receipt.started = true // an uncertain operation must survive the next pruning pass
	done()
	if len(service.receipts) != 1 {
		t.Fatalf("expired terminal records remain: %d", len(service.receipts))
	}
	receipt, done, err = service.receipt(context.Background(), explicitUpload("another", "abc"), []byte("abc"))
	if err != nil {
		t.Fatal(err)
	}
	done()
	if service.receipts["new"] == nil {
		t.Fatal("pruning removed a transfer whose outcome is still unknown")
	}
}

func TestConcurrentCloseWaitsForOwnedReceiptCleanup(t *testing.T) {
	backend := newBackend()
	service, manager, _ := startService(t, backend)
	ctx, cancel := service.operationContext(context.Background())
	defer cancel()
	receipt, done, err := service.receipt(ctx, explicitUpload("active-close", "abc"), []byte("abc"))
	if err != nil {
		t.Fatal(err)
	}
	receipt.started = true
	lease, err := manager.AcquireServer(ctx, serverName)
	if err != nil {
		t.Fatal(err)
	}
	receipt.lease = lease
	closed := make(chan error, 2)
	go func() { closed <- service.Close() }()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("close did not cancel the active operation")
	}
	go func() { closed <- service.Close() }()
	select {
	case <-closed:
		t.Fatal("close returned while the transfer still owned its receipt")
	case <-time.After(20 * time.Millisecond):
	}
	done()
	for range 2 {
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent close did not finish after receipt cleanup")
		}
	}
	if _, err := lease.CallTool(context.Background(), "get_asset_upload_status", nil); err == nil {
		t.Fatal("closed service retained the upload's managed client lease")
	}
}

func TestReceiptBoundsAndCompletedRetention(t *testing.T) {
	service := New(nil)
	service.receipts = make(map[string]*uploadReceipt)
	for index := range maxUploadReceipts {
		service.receipts[string(rune(index+1))] = &uploadReceipt{finishedAt: time.Time{}}
	}
	if _, _, err := service.receipt(context.Background(), explicitUpload("new", "abc"), []byte("abc")); err == nil {
		t.Fatal("uncertain receipt was evicted to start another upload")
	}
	service.receipts["old"] = &uploadReceipt{finishedAt: time.Now().Add(-uploadReceiptTTL - time.Second)}
	receipt, done, err := service.receipt(context.Background(), explicitUpload("new", "abc"), []byte("abc"))
	if err == nil {
		done()
		t.Fatalf("table should still be full after removing one old receipt: %v", receipt)
	}
	delete(service.receipts, string(rune(1)))
	receipt, done, err = service.receipt(context.Background(), explicitUpload("new", "abc"), []byte("abc"))
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if receipt == nil || len(service.receipts) != maxUploadReceipts {
		t.Fatal("receipt capacity not respected")
	}
}
