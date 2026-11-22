package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "myai/core/config"
	"myai/core/mcp"
	"myai/core/trust"
)

func executeTrustForTest(t *testing.T, store *trust.Store, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	command := newTrustCommand(store)
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	err := command.ExecuteContext(context.Background())
	return output.String(), err
}

func TestTrustCLIListsApprovesAndRevokesAllKindsWithoutExposingValues(t *testing.T) {
	workspace := t.TempDir()
	store := trust.NewStore(filepath.Join(t.TempDir(), "trust.json"))
	configPath := filepath.Join(workspace, "declarations.json")
	config := map[string]any{
		"hooks": map[string]any{"commands": []map[string]any{{"event": "pre_tool_use", "command": "unused private-hook-placeholder"}}},
		"mcp":   map[string]any{"servers": []map[string]any{{"name": "dummy", "command": "dummy-helper", "args": []string{"private-arg-placeholder"}, "env": map[string]string{"DUMMY_VALUE": "private-env-placeholder"}, "headers": map[string]string{"Authorization": "private-header-placeholder"}}}},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(workspace, "plugins", "dummy")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{"id":"dummy","entrypoint":"dummy-helper","args":["private-plugin-arg-placeholder"],"env":{"DUMMY_VALUE":"private-plugin-env-placeholder"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []trust.Kind{trust.KindHook, trust.KindPlugin, trust.KindMCP} {
		t.Run(string(kind), func(t *testing.T) {
			requests, err := readTrustRequests(workspace, configPath, "", kind)
			if err != nil || len(requests) != 1 {
				t.Fatalf("requests=%+v err=%v", requests, err)
			}
			request := requests[0]
			base := []string{string(kind), "list", "--workspace", workspace, "--config", configPath}
			output, err := executeTrustForTest(t, store, base...)
			if err != nil || !strings.Contains(output, request.Digest) || !strings.Contains(output, "untrusted") {
				t.Fatalf("list failed: %s %v", output, err)
			}
			if strings.Contains(output, "private-") {
				t.Fatalf("list disclosed declaration values: %s", output)
			}
			base[1] = "approve"
			base = append(base, "--id", request.Digest)
			if _, err := executeTrustForTest(t, store, base...); err != nil {
				t.Fatal(err)
			}
			if approved, err := store.Trusted(request); err != nil || !approved {
				t.Fatalf("CLI approval ineffective: %v", err)
			}
			base[1] = "revoke"
			if _, err := executeTrustForTest(t, store, base...); err != nil {
				t.Fatal(err)
			}
			if approved, err := store.Trusted(request); err != nil || approved {
				t.Fatalf("CLI revocation ineffective: %v", err)
			}
		})
	}
	raw, err = os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-") {
		t.Fatal("approval store contains private declaration values")
	}
}

func TestTrustCLIRevokesApprovalAfterDeclarationRemoved(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "declarations.json")
	if err := os.WriteFile(path, []byte(`{"hooks":{"commands":[{"event":"stop","command":"unused-dummy"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store := trust.NewStore(filepath.Join(t.TempDir(), "trust.json"))
	requests, err := readTrustRequests(workspace, path, "", trust.KindHook)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Approve(requests[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"commands":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := executeTrustForTest(t, store, "hook", "list", "--workspace", workspace, "--config", path)
	if err != nil || !strings.Contains(output, "approved-stale") || !strings.Contains(output, requests[0].Digest) {
		t.Fatalf("stale approval hidden: %s %v", output, err)
	}
	if err := os.WriteFile(path, []byte("invalid configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := executeTrustForTest(t, store, "hook", "revoke", "--workspace", workspace, "--config", path, "--id", requests[0].Digest); err != nil {
		t.Fatal(err)
	}
	if approved, err := store.Trusted(requests[0]); err != nil || approved {
		t.Fatalf("stale approval not revoked: %v", err)
	}
}

func TestTrustCLIRequiresCurrentFullDigest(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "declarations.json")
	if err := os.WriteFile(path, []byte(`{"hooks":{"commands":[{"event":"stop","command":"unused-dummy"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store := trust.NewStore(filepath.Join(t.TempDir(), "trust.json"))
	if _, err := executeTrustForTest(t, store, "approve", "--kind", "hook", "--workspace", workspace, "--config", path, "--id", "123"); err == nil {
		t.Fatal("partial ID accepted")
	}
	requests, err := readTrustRequests(workspace, path, "", trust.KindHook)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"commands":[{"event":"stop","command":"changed-unused-dummy"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := executeTrustForTest(t, store, "approve", "--kind", "hook", "--workspace", workspace, "--config", path, "--id", requests[0].Digest); err == nil {
		t.Fatal("stale digest approved changed command")
	}
}

func TestMCPTrustCLIUsesStartupPathAndEffectiveDeclarations(t *testing.T) {
	deployment := t.TempDir()
	workspace := t.TempDir()
	t.Chdir(deployment)
	if err := os.Mkdir("resource", 0700); err != nil {
		t.Fatal(err)
	}
	const declarations = `mcp:
  servers:
    - name: explicit
      url: ${MYAI_TEST_EXPLICIT_ENDPOINT}
      headers:
        Authorization: "${MYAI_TEST_UNSET_HEADER}"
short_links:
  mcp_url: https://yaml.example.test/api/mcp
`
	t.Setenv("MCP_SERVERS", "")
	t.Setenv("MYAI_TEST_EXPLICIT_ENDPOINT", "https://explicit.example.test/api/mcp?key=fixture-private-marker")
	t.Setenv("MYAI_TEST_UNSET_HEADER", "")
	t.Setenv("SHORT_LINKS_MCP_URL", "https://auto.example.test/api/mcp")
	t.Setenv("SHORT_LINKS_API_KEY", "")
	path := filepath.Join(deployment, "resource", "application.yaml")
	if err := os.WriteFile(path, []byte(declarations), 0600); err != nil {
		t.Fatal(err)
	}
	properties, err := (appconfig.ViperLoader{}).Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	servers := (appconfig.Mapper{}).MCPConfig(workspace, properties.MCP).Servers
	if len(servers) != 2 || servers[1].Name != "short_links" || servers[1].URL != "https://auto.example.test/api/mcp" {
		t.Fatalf("unexpected startup declaration inventory: %#v", servers)
	}
	store := trust.NewStore(filepath.Join(t.TempDir(), "trust.json"))
	output, err := executeTrustForTest(t, store, "mcp", "list", "--workspace", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "fixture-private-marker") || strings.Contains(output, "example.test") || strings.Contains(output, "MYAI_TEST_UNSET_HEADER") {
		t.Fatalf("review leaked declaration values: %s", output)
	}
	for _, server := range servers {
		request, err := mcp.TrustRequest(server, workspace)
		if err != nil || !strings.Contains(output, request.Digest) {
			t.Fatalf("CLI and startup digests differ for %s: %v", server.Name, err)
		}
		if _, err := executeTrustForTest(t, store, "mcp", "approve", "--workspace", workspace, "--config", "resource/application.yaml", "--id", request.Digest); err != nil {
			t.Fatal(err)
		}
		if approved, err := store.Trusted(request); err != nil || !approved {
			t.Fatalf("startup declaration is not approved: %v", err)
		}
	}
	// Unrelated application settings cannot prevent declaration review or reload.
	if err := os.WriteFile(path, []byte(declarations+"rag:\n  embedding:\n    models: invalid-fixture-field\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (appconfig.ViperLoader{}).Load(workspace); err == nil {
		t.Fatal("expected unrelated application decode error")
	}
	if _, err := executeTrustForTest(t, store, "mcp", "list", "--workspace", workspace); err != nil {
		t.Fatalf("MCP-only declaration review failed: %v", err)
	}
}
