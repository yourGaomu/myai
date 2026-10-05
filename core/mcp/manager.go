package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"myai/core/tool"
	tooldef "myai/core/tool/tool"
)

type Manager struct {
	// Manager 管理 MCP 子进程生命周期，并把远程工具包装成项目统一的 Tool 接口。
	config      Config
	mu          sync.Mutex
	operationMu sync.Mutex
	clients     []*Client
	sources     []string
	registry    *tool.RegisterTools
}

func NewManager(config Config) *Manager {
	return &Manager{config: config}
}

func (m *Manager) RegisterAll(ctx context.Context, registry *tool.RegisterTools) error {
	return m.Reload(ctx, m.config, registry)
}

// Reload prepares every configured MCP runtime before publishing any change.
// If a required server cannot start or list its tools, the previous runtime
// and registry sources remain untouched.
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
	m.mu.Unlock()
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
		if strings.TrimSpace(server.Command) == "" {
			cleanupCandidate()
			return fmt.Errorf("mcp server %s command is empty", server.Name)
		}

		// required 服务启动失败会阻止应用启动；可选服务只记录警告并继续。
		client := NewClient(server)
		if err := client.Start(ctx); err != nil {
			err = fmt.Errorf("start mcp server %s failed: %w", server.Name, err)
			if server.Required {
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
			if server.Required {
				cleanupCandidate()
				return err
			}
			log.Printf("warning: %v", err)
			continue
		}

		wrapped := make([]tooldef.Tool, 0, len(infos))
		for _, info := range infos {
			// 工具名加入 server 前缀并处理冲突，避免多个 MCP 暴露同名工具。
			if strings.TrimSpace(info.Name) == "" {
				continue
			}
			exposedName := uniqueToolName(ExposedToolName(server.Name, info.Name), usedNames)
			wrapped = append(wrapped, NewToolWithName(client, server.Name, info, exposedName, server.toolPermission()))
		}

		source := "mcp:" + server.Name
		candidateClients = append(candidateClients, client)
		candidateSources = append(candidateSources, source)
		candidateTools[source] = wrapped
		log.Printf("mcp %s registered %d tools", server.Name, len(wrapped))
	}

	// Publish the complete candidate set in one short registry update. Existing
	// clients are closed only after the new sources are visible.
	releaseTurn := registry.BeginReload()
	defer releaseTurn()
	for source, tools := range candidateTools {
		baseRegistry.RegisterSource(source, tools)
	}
	registry.ReplaceSources(baseRegistry, oldSources, candidateSources)
	m.mu.Lock()
	oldClients := m.clients
	m.config = config
	m.registry = registry
	m.clients = candidateClients
	m.sources = candidateSources
	m.mu.Unlock()
	var closeErrors []error
	for index := len(oldClients) - 1; index >= 0; index-- {
		if oldClients[index] != nil {
			if err := oldClients[index].Close(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
	}
	return errors.Join(closeErrors...)
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
	m.sources = nil
	m.registry = nil
	m.mu.Unlock()
	releaseTurn := func() {}
	if registry != nil {
		releaseTurn = registry.BeginReload()
		defer releaseTurn()
	}

	if registry != nil {
		registry.UnregisterSources(sources)
	}
	var closeErrors []error
	for index := len(clients) - 1; index >= 0; index-- {
		client := clients[index]
		if client == nil {
			continue
		}
		if err := client.Close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
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
