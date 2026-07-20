package langchaingo

import (
	"encoding/json"
	"testing"

	"github.com/tmc/langchaingo/llms"

	domainmessage "myai/core/domain/message"
)

func TestMessageMapperPreservesToolParts(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"),
		domainmessage.Text(domainmessage.RoleUser, "user"),
		domainmessage.ToolCallMessage([]domainmessage.ToolCall{
			{ID: "call-1", Type: "function", Name: "read_file", Arguments: `{"path":"a.go"}`},
		}),
		domainmessage.ToolResultMessage(domainmessage.ToolResult{
			ToolCallID: "call-1",
			Name:       "read_file",
			Content:    "package main",
		}),
	}

	mapped := ToLLMS(messages)
	if len(mapped) != len(messages) {
		t.Fatalf("expected %d messages, got %d", len(messages), len(mapped))
	}
	if mapped[0].Role != llms.ChatMessageTypeSystem {
		t.Fatalf("unexpected system role: %s", mapped[0].Role)
	}
	if text, ok := mapped[0].Parts[0].(llms.TextContent); !ok || text.Text != "system" {
		t.Fatalf("unexpected system content: %#v", mapped[0].Parts)
	}
	call, ok := mapped[2].Parts[0].(llms.ToolCall)
	if !ok {
		t.Fatal("expected tool call")
	}
	if call.ID != "call-1" || call.FunctionCall == nil || call.FunctionCall.Name != "read_file" || call.FunctionCall.Arguments != `{"path":"a.go"}` {
		t.Fatalf("unexpected tool call: %#v", call)
	}
	result, ok := mapped[3].Parts[0].(llms.ToolCallResponse)
	if !ok {
		t.Fatal("expected tool result")
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(result.Content), &output); err != nil {
		t.Fatalf("tool result is not structured JSON: %v", err)
	}
	if result.ToolCallID != "call-1" || result.Name != "read_file" || output["content"] != "package main" || output["status"] != "success" {
		t.Fatalf("unexpected tool result: %#v", result)
	}
}
