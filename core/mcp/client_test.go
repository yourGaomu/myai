package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
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
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(request.ID),
				"result": map[string]any{
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
				},
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
