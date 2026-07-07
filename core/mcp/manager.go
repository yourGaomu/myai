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
	config  Config
	mu      sync.Mutex
	clients []*Client
	sources []string
}

func NewManager(config Config) *Manager {
	return &Manager{config: config}
}

func (m *Manager) RegisterAll(ctx context.Context, registry *tool.RegisterTools) error {
	if registry == nil {
		return errors.New("tool registry is nil")
	}

	for _, server := range m.config.Servers {
		if server.Disabled {
			continue
		}
		if strings.TrimSpace(server.Name) == "" {
			return errors.New("mcp server name is empty")
		}
		if strings.TrimSpace(server.Command) == "" {
			return fmt.Errorf("mcp server %s command is empty", server.Name)
		}

		client := NewClient(server)
		if err := client.Start(ctx); err != nil {
			return fmt.Errorf("start mcp server %s failed: %w", server.Name, err)
		}

		infos, err := client.ListTools(ctx)
		if err != nil {
			_ = client.Close()
			return fmt.Errorf("list mcp tools for %s failed: %w", server.Name, err)
		}

		wrapped := make([]tooldef.Tool, 0, len(infos))
		usedNames := make(map[string]int)
		for _, info := range infos {
			if strings.TrimSpace(info.Name) == "" {
				continue
			}
			exposedName := uniqueToolName(ExposedToolName(server.Name, info.Name), usedNames)
			wrapped = append(wrapped, NewToolWithName(client, server.Name, info, exposedName, server.toolPermission()))
		}

		source := "mcp:" + server.Name
		registry.RegisterSource(source, wrapped)
		m.addRuntime(client, source)
		log.Printf("mcp %s registered %d tools", server.Name, len(wrapped))
	}

	return nil
}

func uniqueToolName(base string, used map[string]int) string {
	if used == nil {
		return base
	}

	count := used[base]
	used[base] = count + 1
	if count == 0 {
		return base
	}

	suffix := fmt.Sprintf("_%d", count+1)
	maxBaseLength := 64 - len(suffix)
	if len(base) > maxBaseLength {
		base = strings.Trim(base[:maxBaseLength], "_-")
	}
	return base + suffix
}

func (m *Manager) Close() error {
	m.mu.Lock()
	clients := append([]*Client(nil), m.clients...)
	m.clients = nil
	m.sources = nil
	m.mu.Unlock()

	var closeErr error
	for _, client := range clients {
		if client == nil {
			continue
		}
		if err := client.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (m *Manager) addRuntime(client *Client, source string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients = append(m.clients, client)
	m.sources = append(m.sources, source)
}
