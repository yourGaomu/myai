package agent

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

func (a *Agent) handleAgentRunList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.AgentRunListPayload](message)
	if err != nil {
		return fmt.Errorf("decode agent run list failed: %w", err)
	}
	sessionID := resolveSessionID(payload.SessionID, message.SessionID, a.chatService.CurrentSessionID())
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}
	snapshots, err := a.chatService.ListAgentRuns(ctx, sessionID, payload.Limit)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeAgentRunListResult, message.RequestID, sessionID, protocol.AgentRunListResultPayload{
		SessionID: sessionID,
		Runs:      agentRunSnapshotsPayload(snapshots),
	})
}
