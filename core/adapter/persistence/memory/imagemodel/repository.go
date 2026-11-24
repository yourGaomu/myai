package imagemodel

import (
	"context"
	"errors"
	"sort"
	"sync"

	domain "myai/core/domain/imagemodel"
)

type Repository struct {
	mu      sync.RWMutex
	configs map[string]domain.Config
}

func NewRepository() *Repository { return &Repository{configs: map[string]domain.Config{}} }
func (r *Repository) Save(_ context.Context, c domain.Config) error {
	r.mu.Lock()
	r.configs[c.ID] = c
	r.mu.Unlock()
	return nil
}
func (r *Repository) Get(_ context.Context, id string) (domain.Config, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.configs[id]
	if !ok {
		return domain.Config{}, errors.New("image model not found")
	}
	return c, nil
}
func (r *Repository) List(context.Context) ([]domain.Config, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Config, 0, len(r.configs))
	for _, c := range r.configs {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *Repository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.configs[id]; !ok {
		return errors.New("image model not found")
	}
	delete(r.configs, id)
	return nil
}
