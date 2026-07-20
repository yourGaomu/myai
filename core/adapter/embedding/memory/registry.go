package memory

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]knowledgeport.EmbeddingProvider
	infos     map[string]domainknowledge.EmbeddingModelInfo
}

var _ knowledgeport.MutableEmbeddingModelRegistry = (*Registry)(nil)

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]knowledgeport.EmbeddingProvider),
		infos:     make(map[string]domainknowledge.EmbeddingModelInfo),
	}
}

func (registry *Registry) Set(modelID string, provider knowledgeport.EmbeddingProvider, info domainknowledge.EmbeddingModelInfo) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return fmt.Errorf("embedding model id is required")
	}
	if provider == nil {
		return fmt.Errorf("embedding provider for model %q is nil", modelID)
	}
	if strings.TrimSpace(info.ID) == "" {
		info.ID = modelID
	}
	if info.ID != modelID {
		return fmt.Errorf("embedding model registry key %q does not match info id %q", modelID, info.ID)
	}
	if strings.TrimSpace(info.Name) == "" {
		info.Name = modelID
	}
	if err := info.Validate(); err != nil {
		return fmt.Errorf("validate embedding model info: %w", err)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.ensureMaps()
	registry.providers[modelID] = provider
	registry.infos[modelID] = info
	return nil
}

func (registry *Registry) Get(modelID string) (knowledgeport.EmbeddingProvider, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	provider, exists := registry.providers[strings.TrimSpace(modelID)]
	return provider, exists
}

func (registry *Registry) GetInfo(modelID string) (domainknowledge.EmbeddingModelInfo, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	info, exists := registry.infos[strings.TrimSpace(modelID)]
	return info, exists
}

func (registry *Registry) List() []domainknowledge.EmbeddingModelInfo {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]domainknowledge.EmbeddingModelInfo, 0, len(registry.infos))
	for _, info := range registry.infos {
		result = append(result, info)
	}
	sort.Slice(result, func(i int, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func (registry *Registry) ensureMaps() {
	if registry.providers == nil {
		registry.providers = make(map[string]knowledgeport.EmbeddingProvider)
	}
	if registry.infos == nil {
		registry.infos = make(map[string]domainknowledge.EmbeddingModelInfo)
	}
}
