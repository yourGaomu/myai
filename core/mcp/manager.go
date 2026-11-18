package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"

	"myai/core/tool"
	tooldef "myai/core/tool/tool"
)

type Manager struct {
	// 1. 管理 stdio 与 HTTP MCP 生命周期，统一复用工具权限、Hook 和热更新。
	config      Config
	mu          sync.Mutex
	operationMu sync.Mutex
	clients     []*Client
	sources     []string
	registry    *tool.RegisterTools
	servers     map[string]*serverRuntime
	revision    uint64
}

type serverRuntime struct {
	client *Client
	tools  map[string]struct{}
}

func NewManager(config Config) *Manager {
	return &Manager{config: config}
}

func (m *Manager) RegisterAll(ctx context.Context, registry *tool.RegisterTools) error {
	return m.Reload(ctx, m.config, registry)
}

// Reload prepares every configured MCP runtime before publishing any change.
// Reload failure preserves the previous runtime, including optional servers.
// During initial startup only, optional server failures can be skipped.
func (m *Manager) Reload(ctx context.Context, config Config, registry *tool.RegisterTools) error {
	if registry == nil {
		return errors.New("tool registry is nil")
	}
	if m == nil {
		return errors.New("mcp manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.operationMu.Lock()
	defer m.operationMu.Unlock()

	m.mu.Lock()
	oldSources := append([]string(nil), m.sources...)
	reloading := m.registry != nil
	registryChanged := reloading && m.registry != registry
	m.mu.Unlock()
	if registryChanged {
		return errors.New("mcp reload requires the existing tool registry")
	}
	prefix := config.SourcePrefix
	if prefix == "" {
		prefix = "mcp:"
	}
	names := make(map[string]bool)
	for _, server := range config.Servers {
		name := strings.TrimSpace(server.Name)
		if name == "" || names[name] {
			return fmt.Errorf("empty or duplicate MCP server name: %q", name)
		}
		names[name] = true
	}
	oldSourceSet := make(map[string]bool, len(oldSources))
	for _, source := range oldSources {
		oldSourceSet[source] = true
	}
	baseRegistry := registry.CloneExcludingSources(oldSourceSet)
	usedNames := make(map[string]int)
	for _, registered := range baseRegistry.List() {
		if registered != nil {
			usedNames[registered.Name()] = 1
		}
	}

	var candidateClients []*Client
	candidateServers := make(map[string]*serverRuntime)
	var candidateSources []string
	var candidateTools = make(map[string][]tooldef.Tool)
	cleanupCandidate := func() {
		for index := len(candidateClients) - 1; index >= 0; index-- {
			_ = candidateClients[index].Close()
		}
	}
	for _, server := range config.Servers {
		if server.Disabled {
			continue
		}
		if strings.TrimSpace(server.Name) == "" {
			cleanupCandidate()
			return errors.New("mcp server name is empty")
		}
		if strings.TrimSpace(server.Command) == "" && strings.TrimSpace(server.URL) == "" {
			cleanupCandidate()
			return fmt.Errorf("mcp server %s requires command or url", server.Name)
		}

		// required 服务启动失败会阻止应用启动；可选服务只记录警告并继续。
		client := NewClient(server)
		if err := client.Start(ctx); err != nil {
			err = fmt.Errorf("start mcp server %s failed: %w", server.Name, err)
			if server.Required || reloading {
				cleanupCandidate()
				return err
			}
			log.Printf("warning: %v", err)
			continue
		}

		infos, err := client.ListTools(ctx)
		if err != nil {
			_ = client.Close()
			err = fmt.Errorf("list mcp tools for %s failed: %w", server.Name, err)
			if server.Required || reloading {
				cleanupCandidate()
				return err
			}
			log.Printf("warning: %v", err)
			continue
		}

		wrapped := make([]tooldef.Tool, 0, len(infos))
		runtime := &serverRuntime{client: client, tools: make(map[string]struct{}, len(infos))}
		for _, info := range infos {
			// Keep original names for host-side calls, including hidden byte tools.
			if strings.TrimSpace(info.Name) == "" {
				continue
			}
			runtime.tools[info.Name] = struct{}{}
			if hiddenModelTool(server.Name, info.Name) {
				continue
			}
			exposedName := uniqueToolName(ExposedToolName(server.Name, info.Name), usedNames)
			wrapped = append(wrapped, NewToolWithName(client, server.Name, info, exposedName, server.toolPermission()))
		}
		candidateServers[strings.TrimSpace(server.Name)] = runtime

		source := prefix + strings.TrimSpace(server.Name)
		candidateClients = append(candidateClients, client)
		candidateSources = append(candidateSources, source)
		candidateTools[source] = wrapped
		log.Printf("mcp %s registered %d tools", server.Name, len(wrapped))
	}

	// Publish the complete candidate set in one short registry update. Existing
	// clients are closed only after the new sources are visible.
	releaseTurn := registry.BeginReload()
	defer releaseTurn()
	if err := ctx.Err(); err != nil {
		cleanupCandidate()
		return err
	}
	for source, tools := range candidateTools {
		baseRegistry.RegisterSource(source, tools)
	}
	registry.ReplaceSources(baseRegistry, oldSources, candidateSources)
	m.mu.Lock()
	oldClients := m.clients
	m.config = config
	m.registry = registry
	m.clients = candidateClients
	m.servers = candidateServers
	m.sources = candidateSources
	m.revision++
	m.mu.Unlock()
	registry.Retire(func() { closeClients(oldClients) })
	return nil
}

// ServerLease pins one managed MCP runtime across a host-side operation. Release
// it after the entire multipart transfer so reload cannot replace its session.
// It intentionally bypasses model tool exposure and approval; callers must apply
// their own user-facing authorization before invoking side effects.
type ServerLease struct {
	mu      sync.RWMutex
	runtime *serverRuntime
	release func()
}

// AcquireServer uses the registry's existing Snapshot/Retire lifetime protocol.
// Manager locks are never held while acquiring a snapshot or doing network I/O.
func (m *Manager) AcquireServer(ctx context.Context, serverName string) (*ServerLease, error) {
	if m == nil {
		return nil, errors.New("mcp manager is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m.mu.Lock()
		registry, revision := m.registry, m.revision
		m.mu.Unlock()
		if registry == nil {
			return nil, errors.New("mcp manager is not initialized")
		}
		_, release := registry.Snapshot()
		m.mu.Lock()
		valid := m.registry == registry && m.revision == revision
		runtime := m.servers[strings.TrimSpace(serverName)]
		m.mu.Unlock()
		if !valid {
			release()
			continue
		}
		if runtime == nil {
			release()
			return nil, errors.New("managed mcp server is unavailable")
		}
		return &ServerLease{runtime: runtime, release: release}, nil
	}
}

// CallTool calls a tool by its original server name and preserves structuredContent.
func (l *ServerLease) CallTool(ctx context.Context, name string, arguments json.RawMessage) (CallResult, error) {
	if l == nil {
		return CallResult{}, errors.New("mcp server lease is unavailable")
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.runtime == nil {
		return CallResult{}, errors.New("mcp server lease is closed")
	}
	if _, ok := l.runtime.tools[name]; !ok {
		return CallResult{}, errors.New("original mcp tool is unavailable")
	}
	return l.runtime.client.CallTool(ctx, name, arguments)
}

func (l *ServerLease) HasTool(name string) bool {
	if l == nil {
		return false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.runtime == nil {
		return false
	}
	_, ok := l.runtime.tools[name]
	return ok
}

// Endpoint returns only the resolved HTTP origin, without headers, userinfo,
// path or query. A file adapter can bind private object hosts to this origin.
func (l *ServerLease) Endpoint() string {
	if l == nil {
		return ""
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.runtime == nil {
		return ""
	}
	endpoint, err := url.Parse(l.runtime.client.httpURL)
	if err != nil || endpoint.Host == "" {
		return ""
	}
	return (&url.URL{Scheme: endpoint.Scheme, Host: endpoint.Host}).String()
}

func (l *ServerLease) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	release := l.release
	l.runtime, l.release = nil, nil
	l.mu.Unlock()
	if release != nil {
		release()
	}
	return nil
}

// CallTool invokes an original tool on a named managed server for a single call.
// Multi-call transfers should acquire one ServerLease instead.
func (m *Manager) CallTool(ctx context.Context, serverName, toolName string, arguments json.RawMessage) (CallResult, error) {
	lease, err := m.AcquireServer(ctx, serverName)
	if err != nil {
		return CallResult{}, err
	}
	defer lease.Close()
	return lease.CallTool(ctx, toolName, arguments)
}

func uniqueToolName(base string, used map[string]int) string {
	if used == nil {
		return base
	}

	count, exists := used[base]
	if !exists {
		used[base] = 1
		return base
	}

	for sequence := count + 1; ; sequence++ {
		suffix := fmt.Sprintf("_%d", sequence)
		candidateBase := base
		maxBaseLength := 64 - len(suffix)
		if len(candidateBase) > maxBaseLength {
			candidateBase = strings.Trim(candidateBase[:maxBaseLength], "_-")
		}
		candidate := candidateBase + suffix
		if _, candidateExists := used[candidate]; candidateExists {
			continue
		}
		used[base] = sequence
		used[candidate] = 1
		return candidate
	}
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	m.mu.Lock()
	clients := append([]*Client(nil), m.clients...)
	sources := append([]string(nil), m.sources...)
	registry := m.registry
	m.clients = nil
	m.servers = nil
	m.sources = nil
	m.registry = nil
	m.revision++
	m.mu.Unlock()
	releaseTurn := func() {}
	if registry != nil {
		releaseTurn = registry.BeginReload()
		defer releaseTurn()
	}

	if registry != nil {
		registry.UnregisterSources(sources)
	}
	if registry != nil {
		registry.Retire(func() { closeClients(clients) })
	} else {
		closeClients(clients)
	}
	return nil
}

func closeClients(clients []*Client) {
	for _, client := range clients {
		if client != nil {
			_ = client.Close()
		}
	}
}

// Sources returns the registry source names owned by this runtime.
func (m *Manager) Sources() []string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.sources...)
}
