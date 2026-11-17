package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

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

func (a *Agent) isSessionPermissionAllowed(sessionID, toolName, permission string) bool {
	if a == nil || a.runtimes == nil {
		return false
	}
	sid := a.resolvePermissionSessionID(sessionID, "")
	return a.runtimes.get(sid).isPermissionAllowed(toolName, permission)
}

func (a *Agent) recordSessionPermissionRule(target permissionRuleTarget, fallbackSessionID, scope string) {
	if a == nil || a.runtimes == nil {
		return
	}
	if scope != "session_tool" && scope != "session_permission" {
		return
	}
	sid := a.resolvePermissionSessionID(target.sessionID, fallbackSessionID)
	a.runtimes.get(sid).allowPermissionRule(scope, target.toolName, target.permission)
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
		if err := control.ResolveBackgroundPermission(ctx, message.SessionID, message.RequestID, payload.ApprovalID, payload.Allowed); err != nil {
			return err
		}
		if a.permissionWaiters != nil {
			key := bgPermissionKey(a.resolvePermissionSessionID(message.SessionID, ""), message.RequestID, payload.ApprovalID)
			if target, found := a.permissionWaiters.consumeBackground(key); found && payload.Allowed {
				a.recordSessionPermissionRule(target, message.SessionID, payload.Scope)
			}
		}
		return nil
	}
	return fmt.Errorf("permission approval is unknown, expired or already answered")
}

func (a *Agent) askToolPermission(ctx context.Context, conn *websocket.Conn, message protocol.Message, request llm.ToolPermissionRequest, sendErrCh chan<- error) bool {
	if a.isSessionPermissionAllowed(message.SessionID, request.Name, string(request.Permission)) {
		return true
	}
	if message.RequestID == "" {
		return false
	}

	approvalID := uuid.NewString()
	key := message.RequestID + ":" + approvalID
	ch := a.permissionWaiters.registerWithTarget(key, permissionRuleTarget{
		sessionID:  message.SessionID,
		toolName:   request.Name,
		permission: string(request.Permission),
	})
	defer a.permissionWaiters.unregister(key, ch)

	if err := a.writeRemoteMessage(conn, protocol.TypePermissionAsk, message.RequestID, message.SessionID, protocol.PermissionAskPayload{
		ApprovalID: approvalID,
		Name:       request.Name,
		Arguments:  request.Arguments,
		Permission: string(request.Permission),
	}); err != nil {
		select {
		case sendErrCh <- err:
		default:
		}
		return false
	}

	timer := time.NewTimer(a.permissionTimeout)
	defer timer.Stop()

	select {
	case allowed := <-ch:
		return allowed
	case <-timer.C:
		log.Printf("tool permission timed out: request=%s tool=%s", message.RequestID, request.Name)
		return false
	case <-ctx.Done():
		return false
	}
}
