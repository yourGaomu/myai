package memoryStore

import (
	"context"
	"myai-url-shortener/internal/shortener"
	"myai-url-shortener/internal/shortener/store"
	"sync"
	"time"
)

type MemoryStore struct {
	mu    sync.RWMutex
	links map[string]shortener.Link
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{links: make(map[string]shortener.Link)}
}

func (s *MemoryStore) Create(ctx context.Context, link shortener.Link) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.links[link.Code]; ok {
		return store.ErrCodeExists
	}
	s.links[link.Code] = link
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, code string) (shortener.Link, error) {
	if err := ctx.Err(); err != nil {
		return shortener.Link{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	link, ok := s.links[code]
	if !ok || link.IsDeleted {
		return shortener.Link{}, store.ErrLinkNotFound
	}
	return link, nil
}

func (s *MemoryStore) IncrementVisits(ctx context.Context, code string) (shortener.Link, error) {
	if err := ctx.Err(); err != nil {
		return shortener.Link{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.links[code]
	if !ok || link.IsDeleted {
		return shortener.Link{}, store.ErrLinkNotFound
	}
	link.Visits++
	link.UpdatedAt = time.Now()
	s.links[code] = link
	return link, nil
}

func (s *MemoryStore) IncrementVisitsBy(ctx context.Context, code string, delta int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delta <= 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.links[code]
	if !ok || link.IsDeleted {
		return store.ErrLinkNotFound
	}
	link.Visits += delta
	link.UpdatedAt = time.Now()
	s.links[code] = link
	return nil
}

func (s *MemoryStore) List(ctx context.Context) ([]shortener.Link, error) {
	if err := ctx.Err(); err != nil {
		return []shortener.Link{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	links := make([]shortener.Link, 0, len(s.links))
	for _, link := range s.links {
		if link.IsDeleted {
			continue
		}
		links = append(links, link)
	}
	return links, nil
}

func (s *MemoryStore) Delete(ctx context.Context, code string) (shortener.Link, error) {
	if err := ctx.Err(); err != nil {
		return shortener.Link{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	link, ok := s.links[code]
	if !ok || link.IsDeleted {
		return shortener.Link{}, store.ErrLinkNotFound
	}
	link.IsDeleted = true
	link.UpdatedAt = time.Now()
	s.links[code] = link
	return link, nil
}
