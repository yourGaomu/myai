package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"myai/core/mcp"
	modelport "myai/core/port/model"
	"myai/core/tool"
	tooldef "myai/core/tool/tool"
	"myai/core/trust"
)

var errPluginUnavailable = errors.New("plugin is disabled or no longer available")

type accessState uint32

const (
	accessAllowed accessState = iota
	accessUnavailable
	accessRevoked
)

// pluginAccess is shared by successive runtimes for one continuously published
// plugin, so withdrawal also reaches snapshots from earlier reloads.
type pluginAccess struct{ state atomic.Uint32 }

func (a *pluginAccess) stop(state accessState) {
	a.state.CompareAndSwap(uint32(accessAllowed), uint32(state))
}

func (a *pluginAccess) check() error {
	switch accessState(a.state.Load()) {
	case accessRevoked:
		return trust.ErrUntrusted
	case accessUnavailable:
		return errPluginUnavailable
	default:
		return nil
	}
}

type pluginRuntime struct {
	manager *mcp.Manager
	info    Info
	grant   trust.Grant
	access  *pluginAccess
	revoked atomic.Bool

	callMu  sync.Mutex
	active  int
	retired bool
}

func (r *pluginRuntime) checkGrant(ctx context.Context) error {
	if r.revoked.Load() {
		return trust.ErrUntrusted
	}
	if err := r.grant.Check(ctx); err != nil {
		if ctx.Err() == nil {
			r.revoked.Store(true)
		}
		return err
	}
	return nil
}

func (r *pluginRuntime) guardTools(registry *tool.RegisterTools) {
	for _, source := range r.manager.Sources() {
		tools := registry.SourceTools(source)
		for i, registered := range tools {
			tools[i] = &guardedTool{Tool: registered, runtime: r, source: source}
		}
		registry.RegisterSource(source, tools)
	}
}

func (r *pluginRuntime) beginCall(ctx context.Context) error {
	if err := r.access.check(); err != nil {
		return err
	}
	if err := r.checkGrant(ctx); err != nil {
		return err
	}
	r.callMu.Lock()
	defer r.callMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	// Recheck shared state after reading the trust store. This is the call's
	// admission point relative to withdrawal, before the underlying MCP call.
	if err := r.access.check(); err != nil {
		return err
	}
	if r.revoked.Load() {
		return trust.ErrUntrusted
	}
	if r.retired {
		return errPluginUnavailable
	}
	r.active++
	return nil
}

func (r *pluginRuntime) endCall() {
	r.callMu.Lock()
	r.active--
	closeNow := r.retired && r.active == 0
	r.callMu.Unlock()
	if closeNow {
		_ = r.manager.Close()
	}
}

// close runs after the registry snapshot leases drain. Direct tool calls that
// were admitted without a snapshot also keep the process alive until they end.
func (r *pluginRuntime) close() error {
	r.callMu.Lock()
	if r.retired {
		r.callMu.Unlock()
		return nil
	}
	r.retired = true
	closeNow := r.active == 0
	r.callMu.Unlock()
	if closeNow {
		return r.manager.Close()
	}
	return nil
}

type guardedTool struct {
	tooldef.Tool
	runtime *pluginRuntime
	source  string
}

func (t *guardedTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if err := t.runtime.beginCall(ctx); err != nil {
		return tooldef.ToolOutput{}, err
	}
	defer t.runtime.endCall()
	return t.Tool.Call(ctx, args)
}

func (t *guardedTool) ModelToolOrigin() modelport.ToolOrigin {
	if source, ok := t.Tool.(interface{ ModelToolOrigin() modelport.ToolOrigin }); ok {
		return source.ModelToolOrigin()
	}
	return 0
}

func (t *guardedTool) RequiresExplicitApproval() bool {
	approval, ok := t.Tool.(tooldef.ExplicitApproval)
	return ok && approval.RequiresExplicitApproval()
}

func (t *guardedTool) PermissionIdentity() string {
	tool, ok := t.Tool.(interface{ PermissionIdentity() string })
	if !ok {
		return ""
	}
	identity := tool.PermissionIdentity()
	if identity == "" {
		return ""
	}
	material, _ := json.Marshal([3]string{t.source, t.runtime.info.TrustRequest.Digest, identity})
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:])
}

// TrustRequest binds the complete normalized manifest and resolved launch paths
// to a workspace. Environment values participate in the digest but are never
// written to the user's approval file.
func TrustRequest(manifest Manifest, workspace, directory string) (trust.Request, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return trust.Request{}, err
	}
	manifest = manifest.normalized()
	return launchTrustRequest(manifest, workspace, directory, manifest.serverConfig(directory))
}

func launchTrustRequest(manifest Manifest, workspace, directory string, server mcp.ServerConfig) (trust.Request, error) {
	return trust.NewRequest(trust.KindPlugin, workspace, filepath.Join(directory, ManifestFileName), struct {
		Manifest   Manifest `json:"manifest"`
		Command    string   `json:"command"`
		WorkingDir string   `json:"working_dir"`
	}{manifest, server.Command, server.WorkingDir})
}

// ReviewRequests discovers declarations for the user-facing approval command. It
// never connects to a plugin or starts its executable.
func ReviewRequests(root, workspace string) ([]trust.Request, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("scan plugin declarations")
	}
	var requests []trust.Request
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(directory, ManifestFileName)); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, errors.New("read plugin declaration")
		}
		manifest, err := loadManifest(directory)
		if err != nil {
			return nil, fmt.Errorf("invalid plugin manifest in %s", directory)
		}
		request, err := TrustRequest(manifest, workspace, directory)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}
