package memory

import (
	"errors"
	"sort"
	"strings"
	"sync"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

type Registry struct {
	mu    sync.RWMutex
	items map[string]domainsubagent.Definition
}

var _ subagentport.DefinitionRegistry = (*Registry)(nil)

func NewRegistry() *Registry {
	return &Registry{items: make(map[string]domainsubagent.Definition)}
}

func (registry *Registry) Get(definitionID string) (domainsubagent.Definition, bool) {
	if registry == nil {
		return domainsubagent.Definition{}, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	definition, ok := registry.items[strings.TrimSpace(definitionID)]
	return domainsubagent.CloneDefinition(definition), ok
}

func (registry *Registry) List() []domainsubagent.Definition {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	items := make([]domainsubagent.Definition, 0, len(registry.items))
	for _, definition := range registry.items {
		items = append(items, domainsubagent.CloneDefinition(definition))
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items
}

func (registry *Registry) Register(definition domainsubagent.Definition) error {
	if registry == nil {
		return errors.New("subagent registry is nil")
	}
	definition = definition.Normalized()
	if err := definition.Validate(); err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.items == nil {
		registry.items = make(map[string]domainsubagent.Definition)
	}
	registry.items[definition.ID] = domainsubagent.CloneDefinition(definition)
	return nil
}

func (registry *Registry) Remove(definitionID string) {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	delete(registry.items, strings.TrimSpace(definitionID))
}
