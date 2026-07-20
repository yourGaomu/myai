package agent

import "sync"

type permissionWaiterRegistry struct {
	mu      sync.Mutex
	waiters map[string][]chan bool
}

func newPermissionWaiterRegistry() *permissionWaiterRegistry {
	return &permissionWaiterRegistry{waiters: make(map[string][]chan bool)}
}

func (r *permissionWaiterRegistry) register(requestID string) chan bool {
	waiter := make(chan bool, 1)
	r.mu.Lock()
	r.waiters[requestID] = append(r.waiters[requestID], waiter)
	r.mu.Unlock()
	return waiter
}

func (r *permissionWaiterRegistry) resolve(requestID string, allowed bool) bool {
	r.mu.Lock()
	waiters := r.waiters[requestID]
	if len(waiters) == 0 {
		r.mu.Unlock()
		return false
	}
	waiter := waiters[0]
	if len(waiters) == 1 {
		delete(r.waiters, requestID)
	} else {
		r.waiters[requestID] = waiters[1:]
	}
	r.mu.Unlock()
	select {
	case waiter <- allowed:
	default:
	}
	return true
}

func (r *permissionWaiterRegistry) unregister(requestID string, waiter chan bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	waiters := r.waiters[requestID]
	for index, current := range waiters {
		if current != waiter {
			continue
		}
		waiters = append(waiters[:index], waiters[index+1:]...)
		if len(waiters) == 0 {
			delete(r.waiters, requestID)
		} else {
			r.waiters[requestID] = waiters
		}
		return
	}
}
