package registry

import (
	"strings"
	"sync"

	subagentport "myai/core/port/subagent"
)

var _ subagentport.AgentPathRegistry = (*Registry)(nil)

// Registry tracks paths for the lifetime of the process. Persisted tasks are
// re-registered during application bootstrap by the owner of the registry.
type Registry struct {
	mu    sync.Mutex
	paths map[string]struct{}
}

func New() *Registry {
	return &Registry{paths: make(map[string]struct{})}
}

func (registry *Registry) Reserve(path string) error {
	path = normalizePath(path)
	if path == "" {
		return nil
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.paths[path]; exists {
		return subagentport.ErrAgentPathTaken
	}
	registry.paths[path] = struct{}{}
	return nil
}

func (registry *Registry) Release(path string) {
	path = normalizePath(path)
	if path == "" {
		return
	}
	registry.mu.Lock()
	delete(registry.paths, path)
	registry.mu.Unlock()
}

func (registry *Registry) Has(path string) bool {
	path = normalizePath(path)
	if path == "" {
		return false
	}
	registry.mu.Lock()
	_, exists := registry.paths[path]
	registry.mu.Unlock()
	return exists
}

func normalizePath(path string) string {
	return strings.Trim(strings.TrimSpace(path), "/")
}
