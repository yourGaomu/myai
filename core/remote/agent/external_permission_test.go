package agent

import (
	"context"
	"testing"
	"time"

	"myai/core/llm"
	"myai/core/remote/protocol"
	"myai/core/service"
	tooldef "myai/core/tool/tool"
)

func TestExternalPermissionUsesCapturedIdentity(t *testing.T) {
	for _, scope := range []string{"session_tool", "session_permission"} {
		t.Run(scope, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			a := &Agent{runtimes: newSessionRuntimeManager(), permissionWaiters: newPermissionWaiterRegistry(), permissionTimeout: 4 * time.Second}
			runtime := a.runtimes.get("session")
			runtime.allowPermissionRule("session_permission", "local_tool", "execute")
			original := llm.ToolPermissionRequest{Name: "mcp_alias", Permission: tooldef.PermissionExecute, PermissionIdentity: "original"}
			replacement := original
			replacement.PermissionIdentity = "replacement"
			asks := make(chan protocol.PermissionAskPayload, 1)
			allowed := make(chan bool, 1)
			go func() {
				allowed <- a.askToolPermission(ctx, "turn", "session", original, func(payload protocol.PermissionAskPayload) error {
					asks <- payload
					return nil
				})
			}()
			var ask protocol.PermissionAskPayload
			select {
			case ask = <-asks:
			case <-ctx.Done():
				t.Fatal("local remembered permission skipped external approval")
			}
			if err := replyToTestPermission(t, a, "turn", "session", ask.ApprovalID, scope, true); err != nil {
				t.Fatal(err)
			}
			if !<-allowed {
				t.Fatal("original call did not receive approval")
			}
			original.Name = "renamed_alias"
			if !a.askToolPermission(ctx, "next", "session", original, nil) {
				t.Fatal("same identity lost its grant after an alias change")
			}
			if a.askToolPermission(ctx, "next", "session", replacement, nil) {
				t.Fatal("replacement inherited approval through the old alias")
			}
			original.RequireFreshConsent = true
			if a.askToolPermission(ctx, "next", "session", original, nil) {
				t.Fatal("fresh approval reused an external grant")
			}
			runtime.clearPermissionRules()
			original.RequireFreshConsent = false
			if a.askToolPermission(ctx, "next", "session", original, nil) {
				t.Fatal("cleared session retained its external grant")
			}
		})
	}
}

func TestBackgroundExternalPermissionUsesCapturedIdentity(t *testing.T) {
	for _, scope := range []string{"session_tool", "session_permission"} {
		t.Run(scope, func(t *testing.T) {
			ctx := context.Background()
			facade := &backgroundControlFacade{permissionPending: true}
			a := &Agent{chatService: facade, runtimes: newSessionRuntimeManager(), permissionWaiters: newPermissionWaiterRegistry()}
			request := llm.ToolPermissionRequest{Name: "mcp_alias", Permission: tooldef.PermissionExecute, PermissionIdentity: "original"}
			event := service.BackgroundTurnEvent{Kind: "permission_ask", SessionID: "session", TurnID: "turn", ApprovalID: "approval", Permission: &request}
			if a.handleBackgroundPermissionEvent(ctx, event) {
				t.Fatal("background request skipped approval")
			}
			request.PermissionIdentity = "replacement"
			if err := replyToTestPermission(t, a, "turn", "session", "approval", scope, true); err != nil {
				t.Fatal(err)
			}
			facade.permissionPending = true
			if a.handleBackgroundPermissionEvent(ctx, event) {
				t.Fatal("replacement inherited the earlier background approval")
			}
			request.PermissionIdentity, request.Name = "original", "renamed_alias"
			if !a.handleBackgroundPermissionEvent(ctx, event) || facade.permissionPending {
				t.Fatal("original background identity did not reuse its grant")
			}
			if a.runtimes.get("session").isPermissionAllowed("unrelated_local_tool", "execute") {
				t.Fatal("external approval granted a local permission category")
			}
		})
	}
}
