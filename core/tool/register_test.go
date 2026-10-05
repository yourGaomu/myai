package tool

import (
	"sync/atomic"
	"testing"
)

func TestSnapshotsDoNotBlockReloadAndRetireAfterLastExistingTurn(t *testing.T) {
	registry := NewRegisterTools()
	_, releaseParent := registry.Snapshot()
	_, releaseOther := registry.Snapshot()
	var closed atomic.Bool
	unlock := registry.BeginReload()
	registry.Retire(func() { closed.Store(true) })
	unlock()
	_, releaseChild := registry.Snapshot()
	defer releaseChild()
	if closed.Load() {
		t.Fatal("retired resources closed while old turns are active")
	}
	releaseParent()
	releaseParent() // releasing twice must be harmless
	if closed.Load() {
		t.Fatal("closed before the last old turn ended")
	}
	releaseOther()
	if !closed.Load() {
		t.Fatal("new child turn incorrectly delayed retirement of old resources")
	}
}
