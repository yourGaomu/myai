package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
