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

func (a *Agent) handlePermissionResult(message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.PermissionResultPayload](message)
	if err != nil {
		return fmt.Errorf("decode permission result failed: %w", err)
	}

	if payload.ApprovalID == "" {
		return fmt.Errorf("permission approval_id is required")
	}
	a.permissionWaiters.resolve(message.RequestID+":"+payload.ApprovalID, payload.Allowed)
	return nil
}

func (a *Agent) askToolPermission(ctx context.Context, conn *websocket.Conn, message protocol.Message, request llm.ToolPermissionRequest, sendErrCh chan<- error) bool {
	if message.RequestID == "" {
		return false
	}

	approvalID := uuid.NewString()
	key := message.RequestID + ":" + approvalID
	ch := a.permissionWaiters.register(key)
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
