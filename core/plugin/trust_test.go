package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"myai/core/tool"
	"myai/core/trust"
)

func TestWorkspacePluginApprovalReloadAndRevocation(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "plugins")
	manifest := Manifest{ID: "echo", Entrypoint: os.Args[0], Args: []string{"-test.run=TestPluginMCPHelperProcess"}, Env: map[string]string{"MYAI_PLUGIN_TEST_HELPER": "1"}}
	writeTestManifest(t, root, manifest)
	store := trust.NewStore(filepath.Join(t.TempDir(), "trust.json"))
	manager := NewManager(root, WithWorkspace(workspace), WithTrustPolicy(&trust.Policy{Store: store}))
	defer manager.Close()
	registry := tool.NewRegisterTools()
	ctx := context.Background()
	if err := manager.Load(ctx, registry); err != nil {
		t.Fatal(err)
	}
	items := manager.List()
	if len(items) != 1 || items[0].Status != StatusUntrusted || len(registry.SourceTools("plugin:echo")) != 0 {
		t.Fatalf("unapproved plugin executed: %#v", items)
	}
	request := items[0].TrustRequest
	if err := store.Approve(request); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if items = manager.List(); len(items) != 1 || items[0].Status != StatusLoaded {
		t.Fatalf("approved plugin unavailable: %#v", items)
	}
	registered, err := registry.GetTool("mcp_echo_echo")
	if err != nil {
		t.Fatal(err)
	}
	if output, err := registered.Call(ctx, []byte(`{"message":"safe"}`)); err != nil || output.Content != "safe" {
		t.Fatalf("approved plugin failed: %+v %v", output, err)
	}
	if err := store.Revoke(request); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if items = manager.List(); items[0].Status != StatusUntrusted || len(registry.SourceTools("plugin:echo")) != 0 {
		t.Fatal("revoked plugin remained admitted")
	}
	if err := store.Approve(request); err != nil {
		t.Fatal(err)
	}
	manifest.Args = append(manifest.Args, "-test.v")
	writeTestManifest(t, root, manifest)
	if err := manager.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if items = manager.List(); items[0].Status != StatusUntrusted || items[0].TrustRequest.Digest == request.Digest {
		t.Fatal("modified declaration reused old approval")
	}
}

func TestPluginConfirmationCannotApproveLaterManifest(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "plugins")
	manifest := Manifest{ID: "dummy", Entrypoint: "unused-dummy-one"}
	writeTestManifest(t, root, manifest)
	policy := &trust.Policy{Store: trust.NewStore(filepath.Join(t.TempDir(), "trust.json")), Confirm: func(context.Context, trust.Request) (trust.Approval, error) {
		manifest.Entrypoint = "unused-dummy-two"
		writeTestManifest(t, root, manifest)
		return trust.Approval{Allowed: true}, nil
	}}
	manager := NewManager(root, WithWorkspace(workspace), WithTrustPolicy(policy))
	defer manager.Close()
	if err := manager.Load(context.Background(), tool.NewRegisterTools()); err != nil {
		t.Fatal(err)
	}
	if items := manager.List(); len(items) != 1 || items[0].Status != StatusUntrusted {
		t.Fatalf("changed manifest admitted: %#v", items)
	}
}

func TestPluginTrustDigestCoversEnvironmentAndPermissions(t *testing.T) {
	workspace := t.TempDir()
	directory := filepath.Join(workspace, "plugins", "dummy")
	manifest := Manifest{ID: "dummy", Entrypoint: "unused-dummy", Env: map[string]string{"DUMMY_VALUE": "first"}}
	first, err := TrustRequest(manifest, workspace, directory)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Env["DUMMY_VALUE"] = "second"
	second, err := TrustRequest(manifest, workspace, directory)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Permission = "read"
	third, err := TrustRequest(manifest, workspace, directory)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest || second.Digest == third.Digest {
		t.Fatal("execution fields excluded from trust digest")
	}
	if permission := (Manifest{ID: "dummy", Entrypoint: "unused-dummy"}).normalized().Permission; permission != "execute" {
		t.Fatalf("unexpected default permission %q", permission)
	}
}
