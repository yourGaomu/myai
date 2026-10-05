package plugin

import (
	"context"
	"encoding/json"
	"myai/core/mcp"
	"myai/core/tool"
	"os"
	"path/filepath"
	"testing"
)

func writeTestManifest(t *testing.T, root string, manifest Manifest) string {
	t.Helper()
	dir := filepath.Join(root, manifest.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ManifestFileName)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPluginNamespaceAndFailedReloadPreserveOtherMCP(t *testing.T) {
	t.Setenv("MYAI_PLUGIN_TEST_HELPER", "1")
	ctx := context.Background()
	registry := tool.NewRegisterTools()
	mcpManager := mcp.NewManager(mcp.Config{Servers: []mcp.ServerConfig{{Name: "echo", Command: os.Args[0], Args: []string{"-test.run=TestPluginMCPHelperProcess"}, Required: true}}})
	if err := mcpManager.RegisterAll(ctx, registry); err != nil {
		t.Fatal(err)
	}
	defer mcpManager.Close()
	root := t.TempDir()
	manifest := Manifest{ID: "echo", Name: "Echo", Entrypoint: os.Args[0], Args: []string{"-test.run=TestPluginMCPHelperProcess"}}
	path := writeTestManifest(t, root, manifest)
	manager := NewManager(root)
	if err := manager.Load(ctx, registry); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if len(registry.SourceTools("mcp:echo")) != 1 || len(registry.SourceTools("plugin:echo")) != 1 {
		t.Fatal("plugin overwrote MCP source")
	}
	old := registry.SourceTools("plugin:echo")[0]
	if err := os.WriteFile(path, []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reload(ctx); err == nil {
		t.Fatal("invalid manifest reload succeeded")
	}
	if registry.SourceTools("plugin:echo")[0] != old {
		t.Fatal("invalid manifest removed running plugin")
	}
	manifest.Entrypoint = filepath.Join(root, "missing")
	writeTestManifest(t, root, manifest)
	if err := manager.Reload(ctx); err == nil {
		t.Fatal("failed optional plugin reload succeeded")
	}
	if registry.SourceTools("plugin:echo")[0] != old {
		t.Fatal("failed startup removed running plugin")
	}
	if err := manager.SetEnabled(ctx, "echo", false); err != nil {
		t.Fatal(err)
	}
	if len(registry.SourceTools("mcp:echo")) != 1 {
		t.Fatal("disabling plugin removed independent MCP")
	}
	if _, err := registry.SourceTools("mcp:echo")[0].Call(ctx, json.RawMessage(`{"message":"still alive"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestFailedOptionalPluginIsNotLoadedAndEnableRollsBack(t *testing.T) {
	root := t.TempDir()
	path := writeTestManifest(t, root, Manifest{ID: "broken", Name: "Broken", Entrypoint: filepath.Join(root, "missing")})
	manager := NewManager(root)
	if err := manager.Load(context.Background(), tool.NewRegisterTools()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if items := manager.List(); len(items) != 1 || items[0].Status != StatusFailed {
		t.Fatalf("incorrect status: %#v", items)
	}
	if err := manager.SetEnabled(context.Background(), "broken", false); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(context.Background(), "broken", true); err == nil {
		t.Fatal("failed enable returned success")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted Manifest
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Enabled == nil || *persisted.Enabled {
		t.Fatal("failed enable was not rolled back")
	}
}
