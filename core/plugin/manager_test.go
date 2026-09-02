package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"myai/core/tool"
)

func TestManagerLoadsMCPPluginAndUnregistersItOnClose(t *testing.T) {
	t.Setenv("MYAI_PLUGIN_TEST_HELPER", "1")

	root := t.TempDir()
	directory := filepath.Join(root, "echo")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		ID:         "echo",
		Name:       "Echo plugin",
		Version:    "1.0.0",
		Entrypoint: os.Args[0],
		Args:       []string{"-test.run=TestPluginMCPHelperProcess"},
		Env:        map[string]string{"MYAI_PLUGIN_TEST_HELPER": "1"},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ManifestFileName), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	registry := tool.NewRegisterTools()
	manager := NewManager(root)
	if err := manager.Load(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	items := manager.List()
	if len(items) != 1 || items[0].Status != StatusLoaded {
		t.Fatalf("unexpected plugin list: %#v", items)
	}

	registered, err := registry.GetTool("mcp_echo_echo")
	if err != nil {
		t.Fatalf("plugin tool was not registered: %v", err)
	}
	arguments, err := json.Marshal(map[string]string{"message": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	output, err := registered.Call(context.Background(), arguments)
	if err != nil {
		t.Fatal(err)
	}
	if output.Content != "hello" {
		t.Fatalf("unexpected plugin output: %#v", output)
	}

	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.GetTool("mcp_echo_echo"); err == nil {
		t.Fatal("plugin tool remained registered after close")
	}
}

func TestManagerReportsDisabledAndInvalidPluginsWithoutStoppingStartup(t *testing.T) {
	root := t.TempDir()
	disabledDirectory := filepath.Join(root, "disabled")
	if err := os.MkdirAll(disabledDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	enabled := false
	disabledManifest, err := json.Marshal(Manifest{ID: "disabled", Name: "Disabled", Entrypoint: "not-used", Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(disabledDirectory, ManifestFileName), disabledManifest, 0o644); err != nil {
		t.Fatal(err)
	}

	invalidDirectory := filepath.Join(root, "invalid")
	if err := os.MkdirAll(invalidDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidDirectory, ManifestFileName), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(root)
	if err := manager.Load(context.Background(), tool.NewRegisterTools()); err != nil {
		t.Fatal(err)
	}
	items := manager.List()
	if len(items) != 2 {
		t.Fatalf("expected disabled and invalid plugin entries, got %#v", items)
	}
	statuses := make(map[string]Status, len(items))
	for _, item := range items {
		statuses[item.Manifest.ID] = item.Status
	}
	if statuses["disabled"] != StatusDisabled || statuses["invalid"] != StatusFailed {
		t.Fatalf("unexpected plugin statuses: %#v", items)
	}
}

func TestManagerSetEnabledPersistsManifestAndReloadsState(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "disabled")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	enabled := false
	manifest, err := json.Marshal(map[string]any{
		"id": "disabled", "name": "Disabled", "entrypoint": "not-used", "enabled": enabled,
		"capabilities": map[string]any{"skills": []string{"future-skill"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, ManifestFileName)
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}

	registry := tool.NewRegisterTools()
	manager := NewManager(root)
	if err := manager.Load(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(context.Background(), "disabled", false); err != nil {
		t.Fatal(err)
	}
	items := manager.List()
	if len(items) != 1 || items[0].Status != StatusDisabled {
		t.Fatalf("expected disabled plugin after toggle, got %#v", items)
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted Manifest
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Enabled == nil || *persisted.Enabled {
		t.Fatalf("expected enabled=false to persist, got %#v", persisted.Enabled)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["capabilities"]; !ok {
		t.Fatal("expected unknown manifest fields to survive enable toggle")
	}
}

func TestPluginMCPHelperProcess(t *testing.T) {
	if os.Getenv("MYAI_PLUGIN_TEST_HELPER") != "1" {
		return
	}
	defer os.Exit(0)

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage
			Method string
			Params json.RawMessage
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil || len(request.ID) == 0 {
			continue
		}
		switch request.Method {
		case "initialize":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(request.ID),
				"result":  map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}},
			})
		case "tools/list":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(request.ID),
				"result": map[string]any{"tools": []map[string]any{{
					"name":        "echo",
					"description": "Echo a message.",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
						"message": map[string]string{"type": "string"},
					}},
				}}},
			})
		case "tools/call":
			var params struct {
				Arguments struct {
					Message string
				}
			}
			_ = json.Unmarshal(request.Params, &params)
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(request.ID),
				"result":  map[string]any{"content": []map[string]string{{"type": "text", "text": params.Arguments.Message}}},
			})
		}
	}
}
