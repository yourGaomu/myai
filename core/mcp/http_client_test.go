package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"myai/core/tool"
)

func shortLinksMCPHandler(t *testing.T, sse, stateful bool) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected MCP endpoint: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path != "/api/mcp" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("X-API-Key") != "test-key" || r.Header.Get("Accept") != "application/json, text/event-stream" {
			t.Error("MCP authentication or Accept header missing")
		}
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Method != "initialize" && (r.Header.Get("MCP-Protocol-Version") != defaultProtocolVersion || stateful && r.Header.Get("Mcp-Session-Id") != "test-session" || !stateful && r.Header.Get("Mcp-Session-Id") != "") {
			t.Error("negotiated MCP protocol or session missing")
		}
		var result any
		switch request.Method {
		case "initialize":
			if stateful {
				w.Header().Set("Mcp-Session-Id", "test-session")
			}
			result = map[string]any{"protocolVersion": defaultProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "short-links-mcp", "version": "1.0.0"}}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "create_url_shortlink", "description": "创建短链", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			params := request.Params.(map[string]any)
			if params["name"] != "create_url_shortlink" || params["arguments"].(map[string]any)["url"] != "https://example.test/page" {
				t.Errorf("unexpected tool arguments: %#v", params)
			}
			result = map[string]any{"content": []map[string]string{{"type": "text", "text": "短链地址：https://links.example.test/r/abc123"}}, "isError": false}
		default:
			t.Errorf("unexpected method: %s", request.Method)
		}
		payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		if sse {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n")
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(payload)
		}
	}
}

func TestHTTPShortLinksRegistrationCallAndReload(t *testing.T) {
	for _, mode := range []struct{ sse, stateful bool }{{false, false}, {false, true}, {true, true}} {
		t.Run(fmt.Sprintf("sse=%t,stateful=%t", mode.sse, mode.stateful), func(t *testing.T) {
			t.Setenv("TEST_SHORT_LINKS_KEY", "test-key")
			server := httptest.NewServer(shortLinksMCPHandler(t, mode.sse, mode.stateful))
			defer server.Close()
			config := NormalizeConfig(Config{Servers: []ServerConfig{{Name: "short_links", URL: server.URL + "/api/mcp", Headers: map[string]string{"X-API-Key": "${TEST_SHORT_LINKS_KEY}"}, Permission: "write", Required: true}}}, "")
			registry := tool.NewRegisterTools()
			manager := NewManager(config)
			if err := manager.RegisterAll(context.Background(), registry); err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			oldSnapshot, release := registry.Snapshot()
			defer release()
			oldTool, err := oldSnapshot.GetTool("mcp_short_links_create_url_shortlink")
			if err != nil {
				t.Fatal(err)
			}
			args := json.RawMessage(`{"url":"https://example.test/page"}`)
			output, err := oldTool.Call(context.Background(), args)
			if err != nil || !strings.Contains(output.Content, "/r/abc123") {
				t.Fatalf("MCP short-link tool failed: %#v %v", output, err)
			}
			broken := config
			broken.Servers = append([]ServerConfig(nil), config.Servers...)
			broken.Servers[0].URL = server.URL + "/missing"
			if err := manager.Reload(context.Background(), broken, registry); err == nil {
				t.Fatal("failed HTTP reload must preserve old tools")
			}
			if _, err := oldTool.Call(context.Background(), args); err != nil {
				t.Fatal(err)
			}
			if err := manager.Reload(context.Background(), config, registry); err != nil {
				t.Fatal(err)
			}
			if _, err := oldTool.Call(context.Background(), args); err != nil {
				t.Fatalf("active snapshot lost its HTTP client: %v", err)
			}
			release()
			if _, err := oldTool.Call(context.Background(), args); err == nil {
				t.Fatal("retired HTTP client still accepts calls")
			}
		})
	}
}

func TestHTTPRejectsUnsafeConfigAndMissingEnvironment(t *testing.T) {
	for _, config := range []ServerConfig{
		{URL: "file:///tmp/mcp"},
		{URL: "https://user:password@example.test/api/mcp"},
		{URL: "https://example.test/api/mcp", Command: "unexpected-process"},
		{URL: "${MYAI_TEST_MISSING_MCP_ENV}"},
		{URL: "https://example.test/api/mcp", Headers: map[string]string{"Authorization": "${MYAI_TEST_MISSING_MCP_ENV}"}},
		{URL: "https://example.test/api/mcp", Headers: map[string]string{"Mcp-Session-Id": "override"}},
		{URL: "https://example.test/api/mcp", Headers: map[string]string{"X-API-Key": "key\ninjected"}},
	} {
		client := NewClient(config)
		if err := client.Start(context.Background()); err == nil {
			client.Close()
			t.Fatal("invalid HTTP configuration was accepted")
		}
	}
}

func TestHTTPDoesNotFollowRedirectsOrRetrySideEffects(t *testing.T) {
	var targetCalls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
	}))
	defer target.Close()
	var sourceCalls atomic.Int64
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceCalls.Add(1)
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client := NewClient(ServerConfig{URL: source.URL, Headers: map[string]string{"X-API-Key": "private-key"}})
	if err := client.Start(context.Background()); err == nil {
		t.Fatal("unexpectedly followed MCP redirect")
	}
	if targetCalls.Load() != 0 || sourceCalls.Load() != 1 {
		t.Fatal("credentials were redirected or request was retried")
	}
}

func TestHTTPValidatesResponsesAndDoesNotLogErrorBody(t *testing.T) {
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":999,"result":{}}`,
		`{"jsonrpc":"2.0","id":1}`,
		`{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":-1,"message":"failed"}}`,
		`invalid JSON`,
		strings.Repeat(" ", maxStdioMessageSize+1),
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, body)
		}))
		client := NewClient(ServerConfig{URL: server.URL})
		if err := client.Start(context.Background()); err == nil {
			client.Close()
			t.Error("invalid MCP response was accepted")
		}
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "private-api-key")
	}))
	defer server.Close()
	client := NewClient(ServerConfig{URL: server.URL})
	err := client.Start(context.Background())
	if err == nil || strings.Contains(err.Error(), "private-api-key") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("unexpected HTTP error: %v", err)
	}
}

func TestHTTPDeadlineAndCloseCancelInFlightRequests(t *testing.T) {
	for _, closeClient := range []bool{false, true} {
		t.Run(fmt.Sprintf("close=%t", closeClient), func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			handler := shortLinksMCPHandler(t, false, true)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Mcp-Session-Id") != "" && r.Header.Get("X-Test-Block") == "yes" {
					_, _ = io.Copy(io.Discard, r.Body)
					close(started)
					select {
					case <-r.Context().Done():
					case <-release:
					}
					return
				}
				handler(w, r)
			}))
			defer server.Close()
			defer close(release)
			client := NewClient(ServerConfig{URL: server.URL + "/api/mcp", Headers: map[string]string{"X-API-Key": "test-key"}})
			if err := client.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			client.httpHeaders.Set("X-Test-Block", "yes")
			client.timeout = 100 * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := client.ListTools(ctx); done <- err }()
			<-started
			if closeClient {
				client.Close()
			}
			select {
			case err := <-done:
				if closeClient && !errors.Is(err, context.Canceled) || !closeClient && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("unexpected cancellation error: %v", err)
				}
			case <-ctx.Done():
				t.Fatal("HTTP request was not cancelled promptly")
			}
		})
	}
}
