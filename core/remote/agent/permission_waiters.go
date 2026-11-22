package agent

import "sync"

type permissionRuleTarget struct {
	sessionID           string
	toolName            string
	permission          string
	identity            string
	requireFreshConsent bool
}

type permissionWaiterRegistry struct {
	mu        sync.Mutex
	waiters   map[string][]chan bool
	targets   map[chan bool]permissionRuleTarget
	bgTargets map[string]permissionRuleTarget
}

func newPermissionWaiterRegistry() *permissionWaiterRegistry {
	return &permissionWaiterRegistry{
		waiters:   make(map[string][]chan bool),
		targets:   make(map[chan bool]permissionRuleTarget),
		bgTargets: make(map[string]permissionRuleTarget),
	}
}

func (r *permissionWaiterRegistry) register(requestID string) chan bool {
	return r.registerWithTarget(requestID, permissionRuleTarget{})
}

func (r *permissionWaiterRegistry) registerWithTarget(requestID string, target permissionRuleTarget) chan bool {
	waiter := make(chan bool, 1)
	r.mu.Lock()
	r.waiters[requestID] = append(r.waiters[requestID], waiter)
	if r.targets == nil {
		r.targets = make(map[chan bool]permissionRuleTarget)
	}
	r.targets[waiter] = target
	r.mu.Unlock()
	return waiter
}

func (r *permissionWaiterRegistry) resolve(requestID string, allowed bool) bool {
	resolved, _ := r.resolveWithTarget(requestID, allowed)
	return resolved
}

func (r *permissionWaiterRegistry) resolveWithTarget(requestID string, allowed bool) (bool, permissionRuleTarget) {
	r.mu.Lock()
	waiters := r.waiters[requestID]
	if len(waiters) == 0 {
		r.mu.Unlock()
		return false, permissionRuleTarget{}
	}
	waiter := waiters[0]
	target := r.targets[waiter]
	delete(r.targets, waiter)
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
	return true, target
}

func (r *permissionWaiterRegistry) unregister(requestID string, waiter chan bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.targets, waiter)
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

func (r *permissionWaiterRegistry) rememberBackground(key string, target permissionRuleTarget) {
	if r == nil || key == "" {
		return
	}
	r.mu.Lock()
	if r.bgTargets == nil {
		r.bgTargets = make(map[string]permissionRuleTarget)
	}
	r.bgTargets[key] = target
	r.mu.Unlock()
}

func (r *permissionWaiterRegistry) backgroundTarget(key string) (permissionRuleTarget, bool) {
	if r == nil || key == "" {
		return permissionRuleTarget{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	target, ok := r.bgTargets[key]
	return target, ok
}

func (r *permissionWaiterRegistry) forgetBackground(key string) {
	if r == nil || key == "" {
		return
	}
	r.mu.Lock()
	delete(r.bgTargets, key)
	r.mu.Unlock()
}
