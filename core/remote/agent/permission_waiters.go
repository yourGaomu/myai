package agent

import "sync"

type permissionWaiterRegistry struct {
	mu      sync.Mutex
	waiters map[string]chan bool
}

func newPermissionWaiterRegistry() *permissionWaiterRegistry {
	return &permissionWaiterRegistry{waiters: make(map[string]chan bool)}
}

func (r *permissionWaiterRegistry) register(requestID string) chan bool {
	waiter := make(chan bool, 1)
	r.mu.Lock()
	r.waiters[requestID] = waiter
	r.mu.Unlock()
	return waiter
}

func (r *permissionWaiterRegistry) resolve(requestID string, allowed bool) bool {
	r.mu.Lock()
	waiter := r.waiters[requestID]
	r.mu.Unlock()
	if waiter == nil {
		return false
	}
	select {
	case waiter <- allowed:
	default:
	}
	return true
}

func (r *permissionWaiterRegistry) unregister(requestID string, waiter chan bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.waiters[requestID] == waiter {
		delete(r.waiters, requestID)
	}
}
