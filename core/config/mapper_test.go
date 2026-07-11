package config

import (
	"testing"
	"time"
)

func TestMapperCreatesIndependentRuntimeConfigs(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	mapper := Mapper{Now: func() time.Time { return now }}

	model := mapper.ModelConfig(ModelProperties{
		ID:      "gpt-test",
		BaseURL: "https://example.test",
		APIKey:  "secret",
	})
	if model.ID != "gpt-test" || model.Provider != "openai" || !model.CreatedAt.Equal(now) {
		t.Fatalf("unexpected model config: %#v", model)
	}

	asset := mapper.AssetConfig(AssetProperties{
		BaseURL:              "https://asset.test",
		UploadTimeoutSeconds: 30,
		TTLSeconds:           60,
	})
	if asset.Timeout != 30*time.Second || asset.DefaultTTLSeconds != 60 {
		t.Fatalf("unexpected asset config: %#v", asset)
	}
}

func TestMapperCreatesIndependentHookAndMCPConfigs(t *testing.T) {
	enabled := true
	mapper := Mapper{}

	hooks := HookProperties{Commands: []CommandHookProperties{{
		Event:   "session_changed",
		Command: "echo changed",
		Enabled: &enabled,
	}}}
	hookConfig := mapper.HookConfig("C:/workspace", hooks)
	if len(hookConfig.CommandHooks) != 1 || hookConfig.CommandHooks[0].Command != "echo changed" {
		t.Fatalf("unexpected hook config: %#v", hookConfig)
	}

	mcpProperties := MCPProperties{Servers: []MCPServerProperties{{
		Name:       "filesystem",
		Command:    "npx",
		Args:       []string{"-y", "server"},
		Env:        map[string]string{"MODE": "test"},
		Permission: "read",
	}}}
	mcpConfig := mapper.MCPConfig("C:/workspace", mcpProperties)
	if len(mcpConfig.Servers) != 1 || mcpConfig.Servers[0].WorkingDir != "C:/workspace" {
		t.Fatalf("unexpected mcp config: %#v", mcpConfig)
	}
	mcpConfig.Servers[0].Args[0] = "changed"
	mcpConfig.Servers[0].Env["MODE"] = "changed"
	if mcpProperties.Servers[0].Args[0] != "-y" || mcpProperties.Servers[0].Env["MODE"] != "test" {
		t.Fatal("expected MCP runtime config to own nested collections")
	}
}
