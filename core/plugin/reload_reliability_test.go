package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"myai/core/mcp"
	"myai/core/tool"
	tooldef "myai/core/tool/tool"
	"myai/core/trust"
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
	mcpManager := mcp.NewManager(mcp.Config{Servers: []mcp.ServerConfig{{Name: "echo", Command: os.Args[0], Args: []string{"-test.run=TestPluginMCPHelperProcess"}, Required: true, Env: map[string]string{"MYAI_PLUGIN_TEST_HELPER": "1"}, Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir()}}})
	if err := mcpManager.RegisterAll(ctx, registry); err != nil {
		t.Fatal(err)
	}
	defer mcpManager.Close()
	root := t.TempDir()
	manifest := Manifest{ID: "echo", Name: "Echo", Entrypoint: os.Args[0], Args: []string{"-test.run=TestPluginMCPHelperProcess"}, Env: map[string]string{"MYAI_PLUGIN_TEST_HELPER": "1"}}
	path := writeTestManifest(t, root, manifest)
	manager := NewManager(root, WithTrustPolicy(&trust.Policy{Allow: true}))
	if err := manager.Load(ctx, registry); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if len(registry.SourceTools("mcp:echo")) != 1 || len(registry.SourceTools("plugin:echo")) != 1 {
		t.Fatal("plugin overwrote MCP source")
	}
	modelTools := registry.LLMTools()
	if len(modelTools) != 2 {
		t.Fatalf("expected direct and plugin MCP definitions, got %d", len(modelTools))
	}
	for _, modelTool := range modelTools {
		if !modelTool.IsMCP() || modelTool.IsBuiltinAssetReader() {
			t.Fatalf("MCP provenance missing after registration for %q", modelTool.Function.Name)
		}
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

func TestPluginWithdrawalSurvivesUnrelatedReloadFailure(t *testing.T) {
	for _, action := range []struct {
		name string
		want Status
	}{
		{"disable", StatusDisabled},
		{"manifest disable", StatusDisabled},
		{"revoke", StatusUntrusted},
	} {
		t.Run(action.name, func(t *testing.T) {
			ctx := context.Background()
			workspace := t.TempDir()
			root := filepath.Join(workspace, "plugins")
			storePath := filepath.Join(t.TempDir(), "trust.json")
			store := trust.NewStore(storePath)
			registry := tool.NewRegisterTools()
			manager := NewManager(root, WithWorkspace(workspace), WithTrustPolicy(&trust.Policy{Store: store}))
			t.Cleanup(func() { _ = manager.Close() })
			manifest := Manifest{
				ID:         "target",
				Entrypoint: os.Args[0],
				Args:       []string{"-test.run=TestPluginMCPHelperProcess"},
				Env:        map[string]string{"MYAI_PLUGIN_TEST_HELPER": "1"},
			}
			paths := make(map[string]string)
			requests := make(map[string]trust.Request)
			for _, id := range []string{"target", "healthy", "aaa-unrelated"} {
				fixture := manifest
				fixture.ID = id
				path := writeTestManifest(t, root, fixture)
				request, err := TrustRequest(fixture, workspace, filepath.Dir(path))
				if err != nil {
					t.Fatal(err)
				}
				if err := store.Approve(request); err != nil {
					t.Fatal(err)
				}
				paths[id], requests[id] = path, request
			}
			if err := manager.Load(ctx, registry); err != nil {
				t.Fatal(err)
			}
			oldest, releaseOldest := registry.Snapshot()
			t.Cleanup(releaseOldest)
			first := fixtureTool(t, oldest, "target")
			if err := manager.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			old, releaseOld := registry.Snapshot()
			t.Cleanup(releaseOld)
			latest := fixtureTool(t, old, "target")
			if latest == first {
				t.Fatal("reload did not replace the tool")
			}
			for _, item := range []tooldef.Tool{first, latest} {
				assertFixtureEcho(t, item)
			}
			healthy := fixtureTool(t, old, "healthy")
			if err := os.WriteFile(paths["aaa-unrelated"], []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}

			wantError := errPluginUnavailable
			switch action.name {
			case "disable":
				if err := manager.SetEnabled(ctx, "target", false); err != nil {
					t.Fatalf("disable should succeed independently: %v", err)
				}
			case "manifest disable":
				disabled := false
				manifest.Enabled = &disabled
				writeTestManifest(t, root, manifest)
			case "revoke":
				wantError = trust.ErrUntrusted
				// An approval for a replacement declaration does not authorize
				// the runtime that was started under the original declaration.
				manifest.Name = "Updated echo"
				writeTestManifest(t, root, manifest)
				replacement, err := TrustRequest(manifest, workspace, filepath.Dir(paths["target"]))
				if err != nil {
					t.Fatal(err)
				}
				if err := store.Approve(replacement); err != nil {
					t.Fatal(err)
				}
				// Separate handle models revocation through the user trust command.
				if err := trust.NewStore(storePath).Revoke(requests["target"]); err != nil {
					t.Fatal(err)
				}
				assertFixtureDenied(t, latest, wantError)
			}
			if err := manager.Reload(ctx); err == nil {
				t.Fatal("reload with an unrelated invalid manifest succeeded")
			}
			if got := registry.SourceTools("plugin:target"); len(got) != 0 {
				t.Fatalf("withdrawn plugin remained in the live registry: %v", got)
			}
			statuses := make(map[string]Status)
			for _, info := range manager.List() {
				statuses[info.Manifest.ID] = info.Status
			}
			if statuses["target"] != action.want || statuses["healthy"] != StatusLoaded {
				t.Fatalf("unexpected status after failed reload: %v", statuses)
			}
			if current := fixtureTool(t, registry, "healthy"); current != healthy {
				t.Fatal("failed reload replaced the healthy runtime")
			}
			assertFixtureEcho(t, healthy)
			if action.want == StatusDisabled {
				current, err := loadManifest(filepath.Dir(paths["target"]))
				if err != nil || current.enabled() {
					t.Fatalf("disable was not persisted: %v", err)
				}
			} else if err := store.Approve(requests["target"]); err != nil {
				t.Fatal(err)
			}
			for _, item := range []tooldef.Tool{first, latest} {
				assertFixtureDenied(t, item, wantError)
			}
			if action.want == StatusUntrusted {
				unrelated := manifest
				unrelated.ID = "aaa-unrelated"
				writeTestManifest(t, root, unrelated)
				if err := manager.Reload(ctx); err != nil {
					t.Fatal(err)
				}
				assertFixtureEcho(t, fixtureTool(t, registry, "target"))
				for _, item := range []tooldef.Tool{first, latest} {
					assertFixtureDenied(t, item, wantError)
				}
			}
		})
	}
}

type blockingFixtureTool struct {
	tooldef.Tool
	started chan struct{}
	resume  chan struct{}
}

func (f blockingFixtureTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	close(f.started)
	select {
	case <-f.resume:
		return f.Tool.Call(ctx, args)
	case <-ctx.Done():
		return tooldef.ToolOutput{}, ctx.Err()
	}
}

func TestDisableLetsAdmittedCallFinishAfterSnapshotReleases(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	writeTestManifest(t, root, Manifest{
		ID:         "echo",
		Entrypoint: os.Args[0],
		Args:       []string{"-test.run=TestPluginMCPHelperProcess"},
		Env:        map[string]string{"MYAI_PLUGIN_TEST_HELPER": "1"},
	})
	registry := tool.NewRegisterTools()
	manager := NewManager(root, WithTrustPolicy(&trust.Policy{Allow: true}))
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.Load(ctx, registry); err != nil {
		t.Fatal(err)
	}
	snapshot, release := registry.Snapshot()
	defer release()
	registered := fixtureTool(t, snapshot, "echo").(*guardedTool)
	fixture := blockingFixtureTool{Tool: registered.Tool, started: make(chan struct{}), resume: make(chan struct{})}
	paused := *registered
	paused.Tool = fixture
	result := make(chan error, 1)
	go func() {
		out, err := paused.Call(ctx, json.RawMessage(`{"message":"finish"}`))
		if err == nil && out.Content != "finish" {
			err = fmt.Errorf("unexpected echo: %q", out.Content)
		}
		result <- err
	}()
	select {
	case <-fixture.started:
	case err := <-result:
		t.Fatalf("call did not reach fixture: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := manager.SetEnabled(ctx, "echo", false); err != nil {
		t.Fatal(err)
	}
	assertFixtureDenied(t, registered, errPluginUnavailable)
	release()
	close(fixture.resume)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("admitted call did not finish: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func fixtureTool(t *testing.T, registry *tool.RegisterTools, id string) tooldef.Tool {
	t.Helper()
	registered := registry.SourceTools("plugin:" + id)
	if len(registered) != 1 {
		t.Fatalf("plugin %q registered %d tools, want one", id, len(registered))
	}
	return registered[0]
}

func assertFixtureEcho(t *testing.T, registered tooldef.Tool) {
	t.Helper()
	output, err := registered.Call(context.Background(), json.RawMessage(`{"message":"hello"}`))
	if err != nil || output.Content != "hello" {
		t.Fatalf("echo = %q, %v", output.Content, err)
	}
}

func assertFixtureDenied(t *testing.T, registered tooldef.Tool, want error) {
	t.Helper()
	output, err := registered.Call(context.Background(), json.RawMessage(`{"message":"hello"}`))
	if !errors.Is(err, want) || output.Content != "" {
		t.Fatalf("withdrawn tool call = %q, %v, want %v", output.Content, err, want)
	}
}

func TestDisableChecksManifestIDBeforeWriting(t *testing.T) {
	root := t.TempDir()
	origID := "original"
	manifest := Manifest{ID: origID, Entrypoint: "unused", Enabled: new(bool)}
	path := writeTestManifest(t, root, manifest)
	manager := NewManager(root, WithTrustPolicy(&trust.Policy{Allow: true}))
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.Load(context.Background(), tool.NewRegisterTools()); err != nil {
		t.Fatal(err)
	}
	changed := []byte(`{"id":"replacement","entrypoint":"unused","extension":"keep"}`)
	if err := os.WriteFile(path, changed, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(context.Background(), origID, false); err == nil {
		t.Fatal("disable accepted a manifest for a different plugin")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != string(changed) {
		t.Fatalf("disable modified a different manifest: %q, %v", raw, err)
	}
}

func TestFailedOptionalPluginIsNotLoadedAndEnableRollsBack(t *testing.T) {
	root := t.TempDir()
	path := writeTestManifest(t, root, Manifest{ID: "broken", Name: "Broken", Entrypoint: filepath.Join(root, "missing")})
	manager := NewManager(root, WithTrustPolicy(&trust.Policy{Allow: true}))
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
