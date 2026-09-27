package registry

import (
	"errors"
	"testing"

	subagentport "myai/core/port/subagent"
)

func TestRegistryReservesAndReleasesNormalizedPaths(t *testing.T) {
	registry := New()
	if err := registry.Reserve(" /root/research/ "); err != nil {
		t.Fatalf("reserve path: %v", err)
	}
	if err := registry.Reserve("root/research"); !errors.Is(err, subagentport.ErrAgentPathTaken) {
		t.Fatalf("expected duplicate path error, got %v", err)
	}
	registry.Release("/root/research")
	if err := registry.Reserve("root/research"); err != nil {
		t.Fatalf("reserve released path: %v", err)
	}
}
