package langchaingo

import (
	"testing"

	domainmessage "myai/core/domain/message"
)

func TestMessageMapperRoundTripPreservesToolParts(t *testing.T) {
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

	roundTrip := FromLLMS(ToLLMS(messages))
	if len(roundTrip) != len(messages) {
		t.Fatalf("expected %d messages, got %d", len(messages), len(roundTrip))
	}
	if roundTrip[0].Role != domainmessage.RoleSystem || roundTrip[0].Text() != "system" {
		t.Fatalf("unexpected system message: %#v", roundTrip[0])
	}
	call, ok := roundTrip[2].FirstToolCall()
	if !ok {
		t.Fatal("expected tool call")
	}
	if call.ID != "call-1" || call.Name != "read_file" || call.Arguments != `{"path":"a.go"}` {
		t.Fatalf("unexpected tool call: %#v", call)
	}
	result, ok := roundTrip[3].FirstToolResult()
	if !ok {
		t.Fatal("expected tool result")
	}
	if result.ToolCallID != "call-1" || result.Name != "read_file" || result.Content != "package main" {
		t.Fatalf("unexpected tool result: %#v", result)
	}
}
