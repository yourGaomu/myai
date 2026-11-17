package mcp

import (
	"encoding/json"
	"testing"

	tooldef "myai/core/tool/tool"
	"myai/core/trust"
)

func TestMCPPermissionDefaultsToExecuteAndUnknownFailsClosed(t *testing.T) {
	for _, permission := range []string{"", "custom"} {
		config := NormalizeConfig(Config{Servers: []ServerConfig{{Name: "demo", URL: "https://example.test", Permission: permission}}}, t.TempDir()).Servers[0]
		want := tooldef.PermissionExecute
		if permission != "" {
			want = tooldef.PermissionUnknown
		}
		if config.toolPermission() != want {
			t.Errorf("permission %q became %q, want %q", permission, config.toolPermission(), want)
		}
	}
	explicit := NormalizeConfig(Config{Servers: []ServerConfig{{Permission: "read"}}}, t.TempDir()).Servers[0]
	if explicit.toolPermission() != tooldef.PermissionRead {
		t.Fatal("explicit trusted read configuration was lost")
	}
}

func TestNormalizeConfigPreservesTrustedInjectionWithoutAliasing(t *testing.T) {
	policy := &trust.Policy{Allow: true}
	input := Config{Servers: []ServerConfig{{Name: "demo", Command: "demo", Args: []string{"version"}, Env: map[string]string{"TEST": "fixture"}, Headers: map[string]string{"X-Test": "fixture"}, Trust: policy, TrustWorkspace: t.TempDir()}}}
	normalized := NormalizeConfig(input, t.TempDir())
	server := normalized.Servers[0]
	if server.Trust == nil || !server.Trust.Allow || server.Trust == policy || server.TrustWorkspace != input.Servers[0].TrustWorkspace {
		t.Fatal("trusted injection not retained as independent policy")
	}
	server.Trust.Allow = false
	server.Args[0] = "other"
	server.Env["TEST"] = "other"
	server.Headers["X-Test"] = "other"
	if !policy.Allow || input.Servers[0].Args[0] != "version" || input.Servers[0].Env["TEST"] != "fixture" || input.Servers[0].Headers["X-Test"] != "fixture" {
		t.Fatal("normalization mutated caller configuration")
	}
	raw, err := json.Marshal(server)
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &encoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := encoded["Trust"]; ok {
		t.Fatal("trusted application policy was serialized")
	}
	if _, ok := encoded["TrustWorkspace"]; ok {
		t.Fatal("trusted application scope was serialized")
	}
}
