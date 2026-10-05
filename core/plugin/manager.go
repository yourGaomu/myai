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
)

type Status string

const (
	StatusLoaded   Status = "loaded"
	StatusDisabled Status = "disabled"
	StatusFailed   Status = "failed"
)

// Info is the user-visible state of a discovered local plugin.
type Info struct {
	Manifest  Manifest
	Directory string
	Status    Status
	Error     string
}

type Manager struct {
	root        string
	operationMu sync.Mutex

	mu       sync.RWMutex
	registry *tool.RegisterTools
	infos    map[string]Info
	runtimes map[string]*mcp.Manager
}

func NewManager(root string) *Manager {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "plugins"
	}
	return &Manager{
		root:     filepath.Clean(root),
		infos:    make(map[string]Info),
		runtimes: make(map[string]*mcp.Manager),
	}
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

func (manager *Manager) load(ctx context.Context, registry *tool.RegisterTools) error {
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
	oldRuntimes := make(map[string]*mcp.Manager, len(manager.runtimes))
	oldSources := make([]string, 0)
	for id, runtime := range manager.runtimes {
		oldRuntimes[id] = runtime
		oldSources = append(oldSources, runtime.Sources()...)
	}
	manager.mu.RUnlock()
	oldSourceSet := make(map[string]bool, len(oldSources))
	for _, source := range oldSources {
		oldSourceSet[source] = true
	}
	candidateRegistry := registry.CloneExcludingSources(oldSourceSet)
	candidateRuntimes := make(map[string]*mcp.Manager)
	candidateInfos := make(map[string]Info)
	cleanupCandidates := func() {
		for _, runtime := range candidateRuntimes {
			_ = runtime.Close()
		}
	}

	entries, err := os.ReadDir(manager.root)
	if errors.Is(err, os.ErrNotExist) {
		releaseTurn := registry.BeginReload()
		registry.ReplaceSources(candidateRegistry, oldSources, nil)
		manager.mu.Lock()
		manager.registry = registry
		manager.infos = candidateInfos
		manager.runtimes = candidateRuntimes
		manager.mu.Unlock()
		closeErr := closeRuntimes(oldRuntimes)
		releaseTurn()
		return closeErr
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
			candidateInfos[entry.Name()] = failedInfo(entry.Name(), directory, statErr.Error())
			continue
		}

		manifest, manifestErr := loadManifest(directory)
		if manifestErr != nil {
			candidateInfos[entry.Name()] = failedInfo(entry.Name(), directory, manifestErr.Error())
			continue
		}
		if _, exists := candidateInfos[manifest.ID]; exists {
			log.Printf("warning: local plugin %s in %s ignored because the id is already registered", manifest.ID, directory)
			continue
		}
		if !manifest.enabled() {
			candidateInfos[manifest.ID] = Info{Manifest: manifest, Directory: directory, Status: StatusDisabled}
			continue
		}

		runtime := mcp.NewManager(mcp.Config{Servers: []mcp.ServerConfig{manifest.serverConfig(directory)}})
		if err := runtime.RegisterAll(ctx, candidateRegistry); err != nil {
			_ = runtime.Close()
			candidateInfos[manifest.ID] = failedInfo(manifest.ID, directory, err.Error())
			if manifest.Required {
				cleanupCandidates()
				return fmt.Errorf("load required plugin %s: %w", manifest.ID, err)
			}
			log.Printf("warning: local plugin %s failed to load: %v", manifest.ID, err)
			continue
		}
		candidateRuntimes[manifest.ID] = runtime
		candidateInfos[manifest.ID] = Info{Manifest: manifest, Directory: directory, Status: StatusLoaded}
		log.Printf("plugin %s loaded from %s", manifest.ID, directory)
	}
	candidateSources := make([]string, 0)
	for _, runtime := range candidateRuntimes {
		candidateSources = append(candidateSources, runtime.Sources()...)
	}
	releaseTurn := registry.BeginReload()
	registry.ReplaceSources(candidateRegistry, oldSources, candidateSources)
	manager.mu.Lock()
	manager.registry = registry
	manager.infos = candidateInfos
	manager.runtimes = candidateRuntimes
	manager.mu.Unlock()
	closeErr := closeRuntimes(oldRuntimes)
	releaseTurn()
	return closeErr
}

func (manager *Manager) Reload(ctx context.Context) error {
	if manager == nil {
		return errors.New("plugin manager is nil")
	}
	manager.mu.RLock()
	registry := manager.registry
	manager.mu.RUnlock()
	if registry == nil {
		return errors.New("plugin tool registry is nil")
	}
	return manager.Load(ctx, registry)
}

// SetEnabled persists a plugin's enabled flag and reloads the plugin set so
// the tool registry and external MCP processes immediately match the manifest.
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
	manifest, err := loadManifest(info.Directory)
	if err != nil {
		return fmt.Errorf("load plugin %s manifest failed: %w", id, err)
	}
	manifest.Enabled = &enabled
	manifestPath := filepath.Join(info.Directory, ManifestFileName)
	// Preserve extension fields (for example future capabilities/skills/hooks)
	// while changing only the lifecycle flag.
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read plugin %s manifest failed: %w", id, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(original, &fields); err != nil {
		return fmt.Errorf("decode plugin %s manifest for update failed: %w", id, err)
	}
	encodedEnabled, err := json.Marshal(enabled)
	if err != nil {
		return fmt.Errorf("encode plugin %s enabled flag failed: %w", id, err)
	}
	fields["enabled"] = encodedEnabled
	raw, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plugin %s manifest update failed: %w", id, err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		return fmt.Errorf("persist plugin %s manifest failed: %w", id, err)
	}
	if err := manager.load(ctx, registry); err != nil {
		// The manifest is a user-facing source of truth. If the candidate reload
		// fails, restore it so a later reload does not silently apply a state that
		// was never successfully activated.
		_ = os.WriteFile(manifestPath, original, 0o644)
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
	runtimes := make(map[string]*mcp.Manager, len(manager.runtimes))
	for id, runtime := range manager.runtimes {
		runtimes[id] = runtime
	}
	sources := make([]string, 0)
	for _, runtime := range runtimes {
		sources = append(sources, runtime.Sources()...)
	}
	registry := manager.registry
	manager.runtimes = make(map[string]*mcp.Manager)
	manager.infos = make(map[string]Info)
	manager.registry = nil
	manager.mu.Unlock()
	if registry != nil {
		releaseTurn := registry.BeginReload()
		registry.UnregisterSources(sources)
		releaseTurn()
	}
	return closeRuntimes(runtimes)
}

func closeRuntimes(runtimes map[string]*mcp.Manager) error {
	items := make([]*mcp.Manager, 0, len(runtimes))
	for _, runtime := range runtimes {
		items = append(items, runtime)
	}
	var closeErrors []error
	for index := len(items) - 1; index >= 0; index-- {
		if runtime := items[index]; runtime != nil {
			if err := runtime.Close(); err != nil {
				closeErrors = append(closeErrors, err)
			}
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
