package agent

import (
	"testing"

	"myai/core/remote/protocol"
)

func TestPermissionRepliesMatchApprovalAndRequest(t *testing.T) {
	a := &Agent{permissionWaiters: newPermissionWaiterRegistry()}
	old := a.permissionWaiters.register("turn:old")
	a.permissionWaiters.unregister("turn:old", old)
	first := a.permissionWaiters.register("turn:first")
	second := a.permissionWaiters.register("turn:second")
	reply := func(request, approval string) error {
		message, err := protocol.NewMessage(protocol.TypePermissionResult, request, "user", "device", "session", protocol.PermissionResultPayload{ApprovalID: approval, Allowed: true})
		if err != nil {
			t.Fatal(err)
		}
		return a.handlePermissionResult(message)
	}
	if err := reply("turn", ""); err == nil {
		t.Fatal("accepted missing approval ID")
	}
	_ = reply("turn", "old")
	_ = reply("other-turn", "first")
	select {
	case <-first:
		t.Fatal("stale or wrong request approved first tool")
	default:
	}
	if err := reply("turn", "second"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-second:
	default:
		t.Fatal("out-of-order reply did not approve second tool")
	}
	select {
	case <-first:
		t.Fatal("second reply approved first tool")
	default:
	}
}
