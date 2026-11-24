package imagemodel

import (
	"errors"
	"sort"
	"strings"
	"sync"

	domain "myai/core/domain/imagemodel"
	port "myai/core/port/imagemodel"
)

type Registry struct {
	mu         sync.RWMutex
	configs    map[string]domain.Config
	generators map[string]port.Generator
	defaultID  string
}

func NewRegistry() *Registry {
	return &Registry{configs: map[string]domain.Config{}, generators: map[string]port.Generator{}}
}
func (r *Registry) Replace(configs []domain.Config, factory port.Factory) error {
	prepared := make(map[string]domain.Config, len(configs))
	generators := make(map[string]port.Generator, len(configs))
	defaultID := ""
	for _, config := range configs {
		config.ID = strings.TrimSpace(config.ID)
		if config.ID == "" {
			return errors.New("image model id is required")
		}
		if _, exists := prepared[config.ID]; exists {
			return errors.New("duplicate image model id")
		}
		prepared[config.ID] = config
		if config.Enabled {
			generator, err := factory.Build(config)
			if err != nil {
				return err
			}
			generators[config.ID] = generator
		}
		if config.Enabled && config.IsDefault {
			if defaultID != "" {
				return errors.New("multiple default image models")
			}
			defaultID = config.ID
		}
	}
	r.mu.Lock()
	r.configs, r.generators, r.defaultID = prepared, generators, defaultID
	r.mu.Unlock()
	return nil
}
func (r *Registry) Get(id string) (domain.Config, port.Generator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	config, ok := r.configs[id]
	generator := r.generators[id]
	return config, generator, ok && config.Enabled && generator != nil
}
func (r *Registry) Default() (domain.Config, port.Generator, bool) {
	r.mu.RLock()
	id := r.defaultID
	r.mu.RUnlock()
	return r.Get(id)
}
func (r *Registry) List() []domain.Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]domain.Config, 0, len(r.configs))
	for _, config := range r.configs {
		result = append(result, config)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
