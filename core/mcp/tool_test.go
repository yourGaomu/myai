package mcp

import (
	"encoding/json"
	"testing"
)

func TestExposedToolNameSanitizesServerAndTool(t *testing.T) {
	got := ExposedToolName("file system", "read/path")
	want := "mcp_file_system_read_path"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExposedToolNameIsShortEnoughForLLMTools(t *testing.T) {
	got := ExposedToolName(
		"very-long-server-name-with-extra-characters",
		"very-long-tool-name-with-extra-characters-that-needs-truncation",
	)
	if len(got) > 64 {
		t.Fatalf("tool name is too long: %q (%d)", got, len(got))
	}
}

func TestUniqueToolNameAddsSuffix(t *testing.T) {
	used := map[string]int{}
	first := uniqueToolName("mcp_server_tool", used)
	second := uniqueToolName("mcp_server_tool", used)
	if first != "mcp_server_tool" {
		t.Fatalf("first name = %q", first)
	}
	if second != "mcp_server_tool_2" {
		t.Fatalf("second name = %q", second)
	}
}

func TestFormatCallResultCombinesContentAndStructuredContent(t *testing.T) {
	result := CallResult{
		Content: []ContentItem{
			{Type: "text", Text: "hello"},
			{Type: "image", MIMEType: "image/png", Data: "abcd"},
		},
		StructuredContent: json.RawMessage(`{"ok":true}`),
	}

	got := formatCallResult(result)
	want := "hello\n\n[image content mime=image/png base64_bytes=4]\n\n{\"ok\":true}"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
