package client

import (
	"testing"

	"myai/core/remote/protocol"
)

func TestPermissionResultDoesNotAutoApprovePerCallConsent(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		autoApprove, fresh, want bool
	}{
		{name: "default denies ordinary"},
		{name: "allow-tools approves ordinary", autoApprove: true, want: true},
		{name: "allow-tools denies per-call consent", autoApprove: true, fresh: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New(Config{AllowTools: tc.autoApprove})
			result := c.permissionResult(protocol.PermissionAskPayload{ApprovalID: "approval", RequireFreshConsent: tc.fresh})
			if result.ApprovalID != "approval" || result.Allowed != tc.want || result.Scope != "" {
				t.Fatalf("unexpected permission result: %#v, want allowed=%v", result, tc.want)
			}
		})
	}
}
