package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func TestClientListAndCallTool(t *testing.T) {
	client := NewClient(ServerConfig{
		Name:           "test",
		Command:        os.Args[0],
		Args:           []string{"-test.run=TestMCPHelperProcess"},
		TimeoutSeconds: 5,
	})
	t.Setenv("MYAI_MCP_TEST_HELPER", "1")

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %#v", tools)
	}

	result, err := client.CallTool(context.Background(), "echo", json.RawMessage(`{"message":"hello"}`))
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if got := formatCallResult(result); got != "hello" {
		t.Fatalf("got %q, want hello", got)
	}
}

func TestClientListToolsRejectsRepeatedCursor(t *testing.T) {
	client := NewClient(ServerConfig{
		Name: "test", Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess"}, TimeoutSeconds: 5,
	})
	t.Setenv("MYAI_MCP_TEST_HELPER", "1")
	t.Setenv("MYAI_MCP_TEST_CURSOR_LOOP", "1")
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer client.Close()
	if _, err := client.ListTools(context.Background()); err == nil {
		t.Fatal("expected repeated tools/list cursor to fail")
	}
}

func TestClientWriteCancellationClosesBlockedStdin(t *testing.T) {
	client := NewClient(ServerConfig{Name: "blocked", TimeoutSeconds: 1})
	writer := newBlockingWriteCloser()
	client.stdin = writer
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := client.writeJSON(ctx, rpcRequest{JSONRPC: "2.0", Method: "test"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	select {
	case <-writer.started:
	default:
		t.Fatal("expected stdin write to start")
	}
	select {
	case <-writer.closed:
	default:
		t.Fatal("expected blocked stdin to be closed on cancellation")
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("MYAI_MCP_TEST_HELPER") != "1" {
		return
	}
	defer os.Exit(0)

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			continue
		}
		if len(request.ID) == 0 {
			continue
		}

		switch request.Method {
		case "initialize":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(request.ID),
				"result": map[string]any{
					"protocolVersion": defaultProtocolVersion,
					"capabilities":    map[string]any{},
					"serverInfo": map[string]string{
						"name":    "test-mcp",
						"version": "0.0.1",
					},
				},
			})
		case "tools/list":
			result := map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "Echo a message.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"message": map[string]string{"type": "string"},
							},
						},
					},
				},
			}
			if os.Getenv("MYAI_MCP_TEST_CURSOR_LOOP") == "1" {
				result["nextCursor"] = "repeat"
			}
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(request.ID),
				"result":  result,
			})
		case "tools/call":
			var params struct {
				Name      string `json:"name"`
				Arguments struct {
					Message string `json:"message"`
				} `json:"arguments"`
			}
			_ = json.Unmarshal(request.Params, &params)
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(request.ID),
				"result": map[string]any{
					"content": []map[string]string{
						{"type": "text", "text": params.Arguments.Message},
					},
				},
			})
		}
	}
}

type blockingWriteCloser struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func newBlockingWriteCloser() *blockingWriteCloser {
	return &blockingWriteCloser{started: make(chan struct{}), closed: make(chan struct{})}
}

func (writer *blockingWriteCloser) Write([]byte) (int, error) {
	writer.once.Do(func() { close(writer.started) })
	<-writer.closed
	return 0, errors.New("writer closed")
}

func (writer *blockingWriteCloser) Close() error {
	select {
	case <-writer.closed:
	default:
		close(writer.closed)
	}
	return nil
}
