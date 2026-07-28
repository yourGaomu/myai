package service

import (
	"context"
	"strings"
	"sync"
)

const globalSessionOperationKey = "__global__"

type sessionOperationCoordinator struct {
	mu      sync.Mutex
	entries map[string]*sessionOperationEntry
}

type sessionOperationEntry struct {
	gate chan struct{}
	refs int
}

func newSessionOperationCoordinator() *sessionOperationCoordinator {
	return &sessionOperationCoordinator{entries: make(map[string]*sessionOperationEntry)}
}

func (coordinator *sessionOperationCoordinator) lock(ctx context.Context, sessionID string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	key := strings.TrimSpace(sessionID)
	if key == "" {
		key = globalSessionOperationKey
	}

	coordinator.mu.Lock()
	entry := coordinator.entries[key]
	if entry == nil {
		entry = &sessionOperationEntry{gate: make(chan struct{}, 1)}
		entry.gate <- struct{}{}
		coordinator.entries[key] = entry
	}
	entry.refs++
	coordinator.mu.Unlock()

	select {
	case <-ctx.Done():
		coordinator.releaseReference(key, entry)
		return nil, ctx.Err()
	case <-entry.gate:
		var once sync.Once
		return func() {
			once.Do(func() {
				entry.gate <- struct{}{}
				coordinator.releaseReference(key, entry)
			})
		}, nil
	}
}

func (coordinator *sessionOperationCoordinator) releaseReference(key string, entry *sessionOperationEntry) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && coordinator.entries[key] == entry {
		delete(coordinator.entries, key)
	}
}
