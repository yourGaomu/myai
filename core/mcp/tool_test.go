package mcp

import (
	"encoding/json"
	"reflect"
	"testing"

	llmmapper "myai/core/adapter/llm/langchaingo"
	modelport "myai/core/port/model"
	"myai/core/tool"
	"myai/core/tool/local"
	tooldef "myai/core/tool/tool"
)

func TestRegisteredToolOriginStaysOutOfModelToolDefinitions(t *testing.T) {
	registry := tool.NewRegisterTools()
	mcpTool := NewToolWithName(nil, "fixture", ToolInfo{Name: "original", Description: "fixture tool"}, "plain_alias", tooldef.PermissionExecute)
	assetReader := local.NewReadAssetToolWithFiles(nil)
	registry.RegisterSource("plugin:fixture", []tooldef.Tool{mcpTool})
	registry.Register(assetReader)

	selected := registry.LLMTools()
	if len(selected) != 2 || !selected[0].IsBuiltinAssetReader() || selected[0].IsMCP() || !selected[1].IsMCP() || selected[1].IsBuiltinAssetReader() {
		t.Fatalf("unexpected tool origins: %#v", selected)
	}
	filtered := registry.LLMToolsByPermission(func(permission tooldef.Permission) bool { return permission == tooldef.PermissionRead })
	if len(filtered) != 1 || !filtered[0].IsBuiltinAssetReader() {
		t.Fatalf("unexpected tool origins after permission filtering: %#v", filtered)
	}

	withoutOrigins := make([]modelport.Tool, 0, len(selected))
	for _, definition := range selected {
		withoutOrigins = append(withoutOrigins, modelport.Tool{Type: definition.Type, Function: definition.Function})
	}
	gotJSON, err := json.Marshal(selected)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(withoutOrigins)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) || !reflect.DeepEqual(llmmapper.ToLLMTools(selected), llmmapper.ToLLMTools(withoutOrigins)) {
		t.Fatalf("internal origin changed the tool definitions: %s != %s", gotJSON, wantJSON)
	}

	// The same definitions without implementation provenance must remain unknown.
	registry.RegisterSource("mcp:lookalikes", []tooldef.Tool{unmarkedTool{assetReader}, unmarkedTool{mcpTool}})
	registry.UnregisterSource("local")
	registry.UnregisterSource("plugin:fixture")
	for _, definition := range registry.LLMTools() {
		if definition.IsMCP() || definition.IsBuiltinAssetReader() {
			t.Fatalf("inferred origin for %q from definition or registry source name", definition.Function.Name)
		}
	}
}

type unmarkedTool struct{ tooldef.Tool }

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

func TestUniqueToolNameSkipsReservedSuffix(t *testing.T) {
	used := map[string]int{"mcp_server_tool": 1, "mcp_server_tool_2": 1}
	if got := uniqueToolName("mcp_server_tool", used); got != "mcp_server_tool_3" {
		t.Fatalf("name = %q, want mcp_server_tool_3", got)
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
