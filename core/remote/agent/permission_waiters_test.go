package agent

import "testing"

func TestPermissionWaiterRegistryResolvesAndUnregisters(t *testing.T) {
	registry := newPermissionWaiterRegistry()
	waiter := registry.register("request-1")

	if !registry.resolve("request-1", true) {
		t.Fatal("expected registered waiter to resolve")
	}
	if allowed := <-waiter; !allowed {
		t.Fatal("expected allowed result")
	}
	registry.unregister("request-1", waiter)
	if registry.resolve("request-1", false) {
		t.Fatal("expected unregistered waiter not to resolve")
	}
}

func TestPermissionWaiterRegistryQueuesWaitersForSameRequest(t *testing.T) {
	registry := newPermissionWaiterRegistry()
	first := registry.register("request-1")
	second := registry.register("request-1")

	if !registry.resolve("request-1", true) {
		t.Fatal("expected first waiter to resolve")
	}
	if allowed := <-first; !allowed {
		t.Fatal("expected first waiter result")
	}
	if !registry.resolve("request-1", false) {
		t.Fatal("expected second waiter to resolve")
	}
	if allowed := <-second; allowed {
		t.Fatal("expected second waiter denial")
	}
}
