package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"myai/core/mcp"
	"myai/core/tool"
	"myai/core/trust"
)

type Status string

const (
	StatusLoaded    Status = "loaded"
	StatusDisabled  Status = "disabled"
	StatusFailed    Status = "failed"
	StatusUntrusted Status = "untrusted"
)

// Info is the user-visible state of a discovered local plugin.
type Info struct {
	Manifest     Manifest
	Directory    string
	Status       Status
	Error        string
	TrustRequest trust.Request
}

type Manager struct {
	root        string
	workspace   string
	trustPolicy *trust.Policy
	operationMu sync.Mutex

	mu       sync.RWMutex
	registry *tool.RegisterTools
	infos    map[string]Info
	runtimes map[string]*pluginRuntime
}

type Option func(*Manager)

func WithTrustPolicy(policy *trust.Policy) Option {
	return func(manager *Manager) { manager.trustPolicy = policy }
}

func WithWorkspace(workspace string) Option {
	return func(manager *Manager) { manager.workspace = workspace }
}

func NewManager(root string, options ...Option) *Manager {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "plugins"
	}
	manager := &Manager{
		root:      filepath.Clean(root),
		workspace: filepath.Dir(filepath.Clean(root)),
		infos:     make(map[string]Info),
		runtimes:  make(map[string]*pluginRuntime),
	}
	for _, option := range options {
		if option != nil {
			option(manager)
		}
	}
	if absolute, err := filepath.Abs(manager.root); err == nil {
		manager.root = absolute
	}
	if absolute, err := filepath.Abs(manager.workspace); err == nil {
		manager.workspace = absolute
	}
	return manager
}

func (manager *Manager) Root() string {
	if manager == nil {
		return ""
	}
	return manager.root
}

// Load discovers immediate child directories containing plugin.json and
// starts enabled MCP-compatible plugins. Missing plugin roots are valid and
// simply result in an empty plugin set.
func (manager *Manager) Load(ctx context.Context, registry *tool.RegisterTools) error {
	if manager != nil {
		manager.operationMu.Lock()
		defer manager.operationMu.Unlock()
	}
	return manager.load(ctx, registry)
}

func (manager *Manager) load(ctx context.Context, registry *tool.RegisterTools) (loadErr error) {
	if manager == nil {
		return errors.New("plugin manager is nil")
	}
	if registry == nil {
		return errors.New("plugin tool registry is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	manager.mu.RLock()
	previousRegistry := manager.registry
	manager.mu.RUnlock()
	if previousRegistry != nil && previousRegistry != registry {
		return errors.New("plugin reload requires the existing tool registry")
	}
	// Withdraw revoked/disabled runtimes independently of the candidate build.
	// An unrelated discovery or startup error must not roll back withdrawal.
	if err := manager.withdrawUnavailable(ctx, registry); err != nil {
		return err
	}
	defer func() {
		if loadErr != nil && ctx.Err() == nil {
			// A revocation or disable can arrive during a slow candidate startup.
			_ = manager.withdrawUnavailable(ctx, registry)
		}
	}()

	reloading := previousRegistry != nil
	manager.mu.RLock()
	oldRuntimes := make(map[string]*pluginRuntime, len(manager.runtimes))
	oldSources := make([]string, 0)
	for id, runtime := range manager.runtimes {
		oldRuntimes[id] = runtime
		oldSources = append(oldSources, runtime.manager.Sources()...)
	}
	manager.mu.RUnlock()
	oldSourceSet := make(map[string]bool, len(oldSources))
	for _, source := range oldSources {
		oldSourceSet[source] = true
	}
	candidateRegistry := registry.CloneExcludingSources(oldSourceSet)
	candidateRuntimes := make(map[string]*pluginRuntime)
	candidateInfos := make(map[string]Info)
	cleanupCandidates := func() { _ = closeRuntimes(candidateRuntimes) }

	entries, err := os.ReadDir(manager.root)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, os.ErrNotExist) {
		return manager.publish(ctx, registry, candidateRegistry, oldSources, oldRuntimes, candidateInfos, candidateRuntimes)
	}
	if err != nil {
		return fmt.Errorf("scan plugin root %s: %w", manager.root, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := filepath.Join(manager.root, entry.Name())
		if _, statErr := os.Stat(filepath.Join(directory, ManifestFileName)); errors.Is(statErr, os.ErrNotExist) {
			continue
		} else if statErr != nil {
			if reloading {
				cleanupCandidates()
				return statErr
			}
			candidateInfos[entry.Name()] = failedInfo(entry.Name(), directory, statErr.Error())
			continue
		}

		manifest, manifestErr := loadManifest(directory)
		if manifestErr != nil {
			if reloading {
				cleanupCandidates()
				return manifestErr
			}
			candidateInfos[entry.Name()] = failedInfo(entry.Name(), directory, manifestErr.Error())
			continue
		}
		if _, exists := candidateInfos[manifest.ID]; exists {
			cleanupCandidates()
			return fmt.Errorf("duplicate plugin id %q in %s", manifest.ID, directory)
		}
		if !manifest.enabled() {
			candidateInfos[manifest.ID] = Info{Manifest: manifest, Directory: directory, Status: StatusDisabled}
			continue
		}

		server := manifest.serverConfig(directory)
		request, err := launchTrustRequest(manifest, manager.workspace, directory, server)
		if err != nil {
			cleanupCandidates()
			return err
		}
		grant, err := manager.trustPolicy.AuthorizeWithGrant(ctx, request)
		if err != nil {
			if ctx.Err() != nil {
				cleanupCandidates()
				return ctx.Err()
			}
			candidateInfos[manifest.ID] = Info{Manifest: manifest, Directory: directory, Status: StatusUntrusted, Error: err.Error(), TrustRequest: request}
			continue
		}
		// Re-read after a review callback: only the exact declaration approved
		// above may start, even if the workspace changed during confirmation.
		current, err := loadManifest(directory)
		if err != nil {
			cleanupCandidates()
			return err
		}
		currentRequest, err := TrustRequest(current, manager.workspace, directory)
		if err != nil {
			cleanupCandidates()
			return err
		}
		if currentRequest.Digest != request.Digest {
			candidateInfos[manifest.ID] = Info{Manifest: current, Directory: directory, Status: StatusUntrusted, Error: trust.ErrUntrusted.Error(), TrustRequest: currentRequest}
			continue
		}
		// This exact launch instance passed the plugin declaration gate above.
		// Its MCP transport still applies the shared process/environment policy.
		server.Trust = &trust.Policy{Allow: true}
		server.TrustWorkspace = manager.workspace
		// Always surface startup errors to the plugin manager. Optional plugin
		// policy is applied here, so a failed runtime can never appear loaded.
		server.Required = true
		runtime := mcp.NewManager(mcp.Config{Servers: []mcp.ServerConfig{server}, SourcePrefix: "plugin:"})
		if err := runtime.RegisterAll(ctx, candidateRegistry); err != nil {
			_ = runtime.Close()
			candidateInfos[manifest.ID] = failedInfo(manifest.ID, directory, err.Error())
			if manifest.Required || reloading {
				cleanupCandidates()
				return fmt.Errorf("load plugin %s: %w", manifest.ID, err)
			}
			log.Printf("warning: local plugin %s failed to load: %v", manifest.ID, err)
			continue
		}
		info := Info{Manifest: manifest, Directory: directory, Status: StatusLoaded, TrustRequest: request}
		access := &pluginAccess{}
		if old := oldRuntimes[manifest.ID]; old != nil && old.info.Directory == directory {
			access = old.access
		}
		guarded := &pluginRuntime{manager: runtime, info: info, grant: grant, access: access}
		guarded.guardTools(candidateRegistry)
		candidateRuntimes[manifest.ID] = guarded
		candidateInfos[manifest.ID] = info
		log.Printf("plugin %s loaded from %s", manifest.ID, directory)
	}
	// Approvals can change during tool discovery. Do not publish a runtime that
	// has already lost the grant used to start it.
	for id, runtime := range candidateRuntimes {
		if err := runtime.checkGrant(ctx); err != nil {
			if ctx.Err() != nil {
				cleanupCandidates()
				return ctx.Err()
			}
			candidateRegistry.UnregisterSources(runtime.manager.Sources())
			_ = runtime.close()
			delete(candidateRuntimes, id)
			candidateInfos[id] = untrustedInfo(runtime.info, err)
		}
	}
	if err := manager.publish(ctx, registry, candidateRegistry, oldSources, oldRuntimes, candidateInfos, candidateRuntimes); err != nil {
		cleanupCandidates()
		return err
	}
	return nil
}

func (manager *Manager) publish(ctx context.Context, registry, candidateRegistry *tool.RegisterTools, oldSources []string, oldRuntimes map[string]*pluginRuntime, candidateInfos map[string]Info, candidateRuntimes map[string]*pluginRuntime) error {
	candidateSources := make([]string, 0)
	for _, runtime := range candidateRuntimes {
		candidateSources = append(candidateSources, runtime.manager.Sources()...)
	}
	releaseTurn := registry.BeginReload()
	defer releaseTurn()
	if err := ctx.Err(); err != nil {
		return err
	}
	for id, old := range oldRuntimes {
		if fresh := candidateRuntimes[id]; fresh == nil || fresh.access != old.access {
			old.access.stop(accessUnavailable)
		}
	}
	registry.ReplaceSources(candidateRegistry, oldSources, candidateSources)
	manager.mu.Lock()
	manager.registry = registry
	manager.infos = candidateInfos
	manager.runtimes = candidateRuntimes
	manager.mu.Unlock()
	registry.Retire(func() { _ = closeRuntimes(oldRuntimes) })
	return nil
}

func untrustedInfo(info Info, err error) Info {
	info.Status = StatusUntrusted
	info.Error = err.Error()
	return info
}

func disabledInfo(manifest Manifest, directory string) Info {
	return Info{Manifest: manifest, Directory: directory, Status: StatusDisabled}
}

// withdrawUnavailable runs with operationMu held. It consults the approval
// used by each running plugin, regardless of what the proposed manifest says.
func (manager *Manager) withdrawUnavailable(ctx context.Context, registry *tool.RegisterTools) error {
	unavailable := make(map[string]Info)
	for id, runtime := range manager.runtimes {
		// A malformed edit to a live declaration is a reload failure. A valid
		// explicit disable can be applied without loading anything else.
		if manifest, err := loadManifest(runtime.info.Directory); err == nil && manifest.ID == id && !manifest.enabled() {
			unavailable[id] = disabledInfo(manifest, runtime.info.Directory)
			continue
		}
		if err := runtime.checkGrant(ctx); err != nil {
			if ctx.Err() != nil {
				manager.withdraw(registry, unavailable)
				return ctx.Err()
			}
			unavailable[id] = untrustedInfo(runtime.info, err)
		}
	}
	manager.withdraw(registry, unavailable)
	return ctx.Err()
}

// withdraw publishes just the affected sources and blocks new calls through
// their older snapshots, leaving all other runtimes in place.
func (manager *Manager) withdraw(registry *tool.RegisterTools, infos map[string]Info) {
	if len(infos) == 0 {
		return
	}
	releaseTurn := registry.BeginReload()
	defer releaseTurn()
	manager.mu.Lock()
	retired := make(map[string]*pluginRuntime, len(infos))
	var sources []string
	for id, info := range infos {
		if runtime := manager.runtimes[id]; runtime != nil {
			state := accessUnavailable
			if info.Status == StatusUntrusted {
				state = accessRevoked
			}
			runtime.access.stop(state)
			sources = append(sources, runtime.manager.Sources()...)
			retired[id] = runtime
			delete(manager.runtimes, id)
		}
		manager.infos[id] = info
	}
	registry.UnregisterSources(sources)
	manager.mu.Unlock()
	registry.Retire(func() { _ = closeRuntimes(retired) })
}

func (manager *Manager) Reload(ctx context.Context) error {
	if manager == nil {
		return errors.New("plugin manager is nil")
	}
	manager.operationMu.Lock()
	defer manager.operationMu.Unlock()
	return manager.load(ctx, manager.registry)
}

// SetEnabled persists a plugin's enabled flag. Disable is a standalone
// withdrawal, while enable must pass the candidate reload before publication.
func (manager *Manager) SetEnabled(ctx context.Context, id string, enabled bool) error {
	if manager == nil {
		return errors.New("plugin manager is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("plugin id is required")
	}
	manager.operationMu.Lock()
	defer manager.operationMu.Unlock()

	manager.mu.RLock()
	info, exists := manager.infos[id]
	registry := manager.registry
	manager.mu.RUnlock()
	if !exists {
		return fmt.Errorf("plugin %s was not discovered", id)
	}
	if registry == nil {
		return errors.New("plugin tool registry is nil")
	}
	manifestPath := filepath.Join(info.Directory, ManifestFileName)
	original, _, err := readManifestForUpdate(manifestPath)
	if err != nil {
		return fmt.Errorf("read plugin %s manifest failed: %w", id, err)
	}
	manifest, err := decodeManifest(manifestPath, original)
	if err != nil {
		return fmt.Errorf("load plugin %s manifest failed: %w", id, err)
	}
	if manifest.ID != id {
		return fmt.Errorf("plugin %s manifest no longer declares the requested id", id)
	}
	manifest.Enabled = &enabled
	// Preserve extension fields while changing only the lifecycle flag.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(original, &fields); err != nil {
		return fmt.Errorf("decode plugin %s manifest for update failed: %w", id, err)
	}
	if enabled {
		fields["enabled"] = json.RawMessage("true")
	} else {
		fields["enabled"] = json.RawMessage("false")
	}
	raw, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plugin %s manifest update failed: %w", id, err)
	}
	raw = append(raw, '\n')
	if err := replaceManifest(manifestPath, original, raw); err != nil {
		return fmt.Errorf("persist plugin %s manifest failed: %w", id, err)
	}
	if !enabled {
		manager.withdraw(registry, map[string]Info{id: disabledInfo(manifest, info.Directory)})
		return nil
	}
	if err := manager.load(ctx, registry); err != nil {
		// Roll back a failed enable, preserving any external edits made since.
		if restoreErr := replaceManifest(manifestPath, raw, original); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore plugin manifest: %w", restoreErr))
		}
		return err
	}
	return nil
}

func (manager *Manager) List() []Info {
	if manager == nil {
		return nil
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	items := make([]Info, 0, len(manager.infos))
	for _, info := range manager.infos {
		info.Manifest.Args = append([]string(nil), info.Manifest.Args...)
		info.Manifest.Env = cloneMap(info.Manifest.Env)
		items = append(items, info)
	}
	sort.Slice(items, func(left, right int) bool {
		return strings.ToLower(items[left].Manifest.ID) < strings.ToLower(items[right].Manifest.ID)
	})
	return items
}

func (manager *Manager) Close() error {
	if manager == nil {
		return nil
	}
	manager.operationMu.Lock()
	defer manager.operationMu.Unlock()
	manager.mu.Lock()
	runtimes := manager.runtimes
	sources := make([]string, 0)
	for _, runtime := range runtimes {
		runtime.access.stop(accessUnavailable)
		sources = append(sources, runtime.manager.Sources()...)
	}
	registry := manager.registry
	manager.runtimes = make(map[string]*pluginRuntime)
	manager.infos = make(map[string]Info)
	manager.registry = nil
	manager.mu.Unlock()
	if registry != nil {
		releaseTurn := registry.BeginReload()
		registry.UnregisterSources(sources)
		registry.Retire(func() { _ = closeRuntimes(runtimes) })
		releaseTurn()
		return nil
	}
	return closeRuntimes(runtimes)
}

func closeRuntimes(runtimes map[string]*pluginRuntime) error {
	var closeErrors []error
	for _, runtime := range runtimes {
		if err := runtime.close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
}

func failedInfo(id string, directory string, message string) Info {
	id = strings.TrimSpace(id)
	if id == "" {
		id = filepath.Base(directory)
	}
	return Info{Manifest: Manifest{ID: id, Name: id}, Directory: directory, Status: StatusFailed, Error: strings.TrimSpace(message)}
}

func (manager *Manager) recordInfo(info Info) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.infos == nil {
		manager.infos = make(map[string]Info)
	}
	manager.infos[info.Manifest.ID] = info
}

func (manager *Manager) recordFailure(id string, directory string, message string) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = filepath.Base(directory)
	}
	manager.recordInfo(Info{
		Manifest:  Manifest{ID: id, Name: id},
		Directory: directory,
		Status:    StatusFailed,
		Error:     strings.TrimSpace(message),
	})
}

func (manager *Manager) hasInfo(id string) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	_, exists := manager.infos[strings.TrimSpace(id)]
	return exists
}
