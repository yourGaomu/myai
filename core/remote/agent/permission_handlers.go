package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"myai/core/llm"
	"myai/core/remote/protocol"
)

func bgPermissionKey(sessionID, turnID, approvalID string) string {
	return sessionID + "\x00" + turnID + "\x00" + approvalID
}

func (a *Agent) resolvePermissionSessionID(primary, secondary string) string {
	if primary != "" {
		return primary
	}
	if secondary != "" {
		return secondary
	}
	if a != nil && a.chatService != nil {
		return a.chatService.CurrentSessionID()
	}
	return ""
}

func newPermissionRuleTarget(sessionID string, request llm.ToolPermissionRequest) permissionRuleTarget {
	return permissionRuleTarget{
		sessionID:           sessionID,
		toolName:            request.Name,
		permission:          string(request.Permission),
		identity:            request.PermissionIdentity,
		requireFreshConsent: request.RequireFreshConsent,
	}
}

func permissionAskPayload(approvalID string, request llm.ToolPermissionRequest) protocol.PermissionAskPayload {
	return protocol.PermissionAskPayload{
		ApprovalID:          approvalID,
		Name:                request.Name,
		Arguments:           request.Arguments,
		Permission:          string(request.Permission),
		RequireFreshConsent: request.RequireFreshConsent,
	}
}

func (a *Agent) canReusePermission(target permissionRuleTarget) bool {
	if target.requireFreshConsent || a == nil || a.runtimes == nil {
		return false
	}
	sid := a.resolvePermissionSessionID(target.sessionID, "")
	runtime := a.runtimes.get(sid)
	if target.identity != "" {
		return runtime.isExternalPermissionAllowed(target.identity)
	}
	return runtime.isPermissionAllowed(target.toolName, target.permission)
}

func (a *Agent) recordSessionPermissionRule(target permissionRuleTarget, fallbackSessionID, scope string) {
	// The pending request, not the reply, decides whether the grant can be reused.
	if target.requireFreshConsent || a == nil || a.runtimes == nil {
		return
	}
	if scope != "session_tool" && scope != "session_permission" {
		return
	}
	sid := a.resolvePermissionSessionID(target.sessionID, fallbackSessionID)
	runtime := a.runtimes.get(sid)
	if target.identity != "" {
		runtime.allowExternalPermission(target.identity)
		return
	}
	runtime.allowPermissionRule(scope, target.toolName, target.permission)
}

func (a *Agent) handlePermissionResult(message protocol.Message) error {
	return a.handlePermissionResultContext(context.Background(), message)
}

func (a *Agent) handlePermissionResultContext(ctx context.Context, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.PermissionResultPayload](message)
	if err != nil {
		return fmt.Errorf("decode permission result failed: %w", err)
	}

	if payload.ApprovalID == "" {
		return fmt.Errorf("permission approval_id is required")
	}
	if a.permissionWaiters != nil {
		if resolved, target := a.permissionWaiters.resolveWithTarget(message.RequestID+":"+payload.ApprovalID, payload.Allowed); resolved {
			if payload.Allowed {
				a.recordSessionPermissionRule(target, message.SessionID, payload.Scope)
			}
			return nil
		}
	}
	if control, ok := a.chatService.(BackgroundPermissionControl); ok {
		key := bgPermissionKey(message.SessionID, message.RequestID, payload.ApprovalID)
		// Snapshot the target before resolution can publish permission_resolved
		// and retire it. Record a rule only after the service accepts the reply.
		target, found := a.permissionWaiters.backgroundTarget(key)
		if err := control.ResolveBackgroundPermission(ctx, message.SessionID, message.RequestID, payload.ApprovalID, payload.Allowed); err != nil {
			return err
		}
		a.permissionWaiters.forgetBackground(key)
		if found && payload.Allowed {
			a.recordSessionPermissionRule(target, message.SessionID, payload.Scope)
		}
		return nil
	}
	return fmt.Errorf("permission approval is unknown, expired or already answered")
}

func (a *Agent) askToolPermission(ctx context.Context, requestID, sessionID string, request llm.ToolPermissionRequest, send func(protocol.PermissionAskPayload) error) bool {
	target := newPermissionRuleTarget(sessionID, request)
	if a.canReusePermission(target) {
		return true
	}
	if requestID == "" || a.permissionWaiters == nil || send == nil {
		return false
	}

	approvalID := uuid.NewString()
	key := requestID + ":" + approvalID
	ch := a.permissionWaiters.registerWithTarget(key, target)
	defer a.permissionWaiters.unregister(key, ch)

	if err := send(permissionAskPayload(approvalID, request)); err != nil {
		return false
	}

	timer := time.NewTimer(a.permissionTimeout)
	defer timer.Stop()

	select {
	case allowed := <-ch:
		return allowed
	case <-timer.C:
		log.Printf("tool permission timed out: request=%s tool=%s", requestID, request.Name)
		return false
	case <-ctx.Done():
		return false
	}
}
