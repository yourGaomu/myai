package mcp

import (
	"context"
	"os"
	"testing"

	tooldef "myai/core/tool/tool"
	"myai/core/trust"
)

func TestToolPermissionIdentityTracksDeclarationAndOriginalTool(t *testing.T) {
	config := ServerConfig{
		Name: "fixture", Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess"}, TimeoutSeconds: 5,
		Env: map[string]string{"MYAI_MCP_TEST_HELPER": "1"}, Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(),
	}
	start := func(config ServerConfig) *Client {
		t.Helper()
		client := NewClient(config)
		if err := client.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })
		return client
	}
	first, same := start(config), start(config)
	config.Env = map[string]string{"MYAI_MCP_TEST_HELPER": "1", "MYAI_FIXTURE_DECLARATION": "updated"}
	different := start(config)
	identity := func(client *Client, source, original, alias string, permission tooldef.Permission) string {
		return NewToolWithName(client, config.Name, ToolInfo{Name: original}, alias, permission).WithPermissionSource(source).PermissionIdentity()
	}
	base := identity(first, "mcp:fixture", "echo", "mcp_alias", tooldef.PermissionExecute)
	if base == "" || base != identity(same, "mcp:fixture", "echo", "mcp_other_alias", tooldef.PermissionExecute) {
		t.Fatal("same declaration and original tool should retain their identity across reload and alias changes")
	}
	for name, got := range map[string]string{
		"source":      identity(first, "plugin:fixture", "echo", "mcp_alias", tooldef.PermissionExecute),
		"original":    identity(first, "mcp:fixture", "other", "mcp_alias", tooldef.PermissionExecute),
		"permission":  identity(first, "mcp:fixture", "echo", "mcp_alias", tooldef.PermissionWrite),
		"declaration": identity(different, "mcp:fixture", "echo", "mcp_alias", tooldef.PermissionExecute),
	} {
		if got == "" || got == base {
			t.Errorf("%s change did not change identity", name)
		}
	}
}
