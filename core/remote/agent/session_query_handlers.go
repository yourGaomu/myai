package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

func (a *Agent) handleSessionList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	request, err := protocol.DecodePayload[protocol.SessionListPayload](message)
	if err != nil {
		return fmt.Errorf("decode session list failed: %w", err)
	}

	a.requestMu.Lock()
	defer a.requestMu.Unlock()

	payload, err := a.sessionListPayload(ctx, request.IncludeDeleted)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionListResult, message.RequestID, payload.CurrentSessionID, payload)
}

func (a *Agent) handleSessionNew(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	a.requestMu.Lock()
	defer a.requestMu.Unlock()

	if err := a.chatService.NewSession(ctx); err != nil {
		return err
	}
	return a.writeSessionChanged(ctx, conn, message.RequestID)
}

func (a *Agent) handleSessionLoad(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionLoadPayload](message)
	if err != nil {
		return fmt.Errorf("decode session load failed: %w", err)
	}
	sessionID := payload.SessionID
	if sessionID == "" {
		sessionID = message.SessionID
	}
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}

	a.requestMu.Lock()
	defer a.requestMu.Unlock()

	if err := a.chatService.LoadSession(ctx, sessionID); err != nil {
		return err
	}
	return a.writeSessionChanged(ctx, conn, message.RequestID)
}

func (a *Agent) handleSessionDelete(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionDeletePayload](message)
	if err != nil {
		return fmt.Errorf("decode session delete failed: %w", err)
	}
	sessionID := resolveSessionID(payload.SessionID, message.SessionID, a.chatService.CurrentSessionID())
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}

	runtime := a.runtimes.get(sessionID)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	a.requestMu.Lock()
	defer a.requestMu.Unlock()

	if err := a.chatService.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	payloadResult, err := a.sessionChangedPayload(ctx)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionDeleteResult, message.RequestID, payloadResult.CurrentSessionID, payloadResult)
}

func (a *Agent) handleSessionRestore(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionRestorePayload](message)
	if err != nil {
		return fmt.Errorf("decode session restore failed: %w", err)
	}
	sessionID := resolveSessionID(payload.SessionID, message.SessionID, "")
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}

	runtime := a.runtimes.get(sessionID)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	a.requestMu.Lock()
	defer a.requestMu.Unlock()

	if err := a.chatService.RestoreSession(ctx, sessionID); err != nil {
		return err
	}
	payloadResult, err := a.sessionChangedPayload(ctx)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionRestoreResult, message.RequestID, payloadResult.CurrentSessionID, payloadResult)
}

func (a *Agent) handleSessionHistory(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionHistoryPayload](message)
	if err != nil {
		return fmt.Errorf("decode session history failed: %w", err)
	}
	sessionID := payload.SessionID
	if sessionID == "" {
		sessionID = message.SessionID
	}
	if sessionID == "" {
		sessionID = a.chatService.CurrentSessionID()
	}
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}

	a.requestMu.Lock()
	defer a.requestMu.Unlock()

	records, err := a.chatService.ListSessionMessages(ctx, sessionID)
	if err != nil {
		return err
	}

	payloadResult := protocol.SessionHistoryResultPayload{
		SessionID: sessionID,
		Messages:  sessionHistoryMessages(records),
		Count:     len(records),
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionHistoryResult, message.RequestID, sessionID, payloadResult)
}

func (a *Agent) handleSessionHistoryMeta(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionHistoryMetaPayload](message)
	if err != nil {
		return fmt.Errorf("decode session history meta failed: %w", err)
	}
	sessionID := resolveSessionID(payload.SessionID, message.SessionID, a.chatService.CurrentSessionID())
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}

	meta, err := a.chatService.SessionHistoryMeta(ctx, sessionID)
	if err != nil {
		return err
	}

	upToDate := localHistoryUpToDate(payload, meta)
	result := protocol.SessionHistoryMetaResultPayload{
		SessionID:            sessionID,
		MessageCount:         meta.MessageCount,
		LastMessageID:        meta.LastMessageID,
		LastMessageCreatedAt: meta.LastMessageCreatedAt,
		HistoryVersion:       meta.HistoryVersion,
		UpToDate:             upToDate,
		CanDelta:             !upToDate && strings.TrimSpace(payload.LocalLastMessageID) != "" && int64(payload.LocalMessageCount) <= meta.MessageCount,
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionHistoryMetaResult, message.RequestID, sessionID, result)
}

func (a *Agent) handleSessionHistoryDelta(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionHistoryDeltaPayload](message)
	if err != nil {
		return fmt.Errorf("decode session history delta failed: %w", err)
	}
	sessionID := resolveSessionID(payload.SessionID, message.SessionID, a.chatService.CurrentSessionID())
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}

	records, fullSyncRequired, err := a.chatService.ListSessionMessagesAfter(ctx, sessionID, payload.AfterMessageID, payload.Limit)
	if err != nil {
		return err
	}

	result := protocol.SessionHistoryDeltaResultPayload{
		SessionID:        sessionID,
		Messages:         sessionHistoryMessages(records),
		Count:            len(records),
		FullSyncRequired: fullSyncRequired,
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionHistoryDeltaResult, message.RequestID, sessionID, result)
}

func (a *Agent) handleAssetList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.AssetListPayload](message)
	if err != nil {
		return fmt.Errorf("decode asset list failed: %w", err)
	}
	sessionID := resolveSessionID(payload.SessionID, message.SessionID, a.chatService.CurrentSessionID())
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}

	records, err := a.chatService.ListAssets(ctx, sessionID, payload.Limit)
	if err != nil {
		return err
	}
	result := protocol.AssetListResultPayload{
		SessionID: sessionID,
		Assets:    assetSummaries(records),
		Count:     len(records),
	}
	return a.writeRemoteMessage(conn, protocol.TypeAssetListResult, message.RequestID, sessionID, result)
}

func (a *Agent) writeSessionChanged(ctx context.Context, conn *websocket.Conn, requestID string) error {
	payload, err := a.sessionChangedPayload(ctx)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionChanged, requestID, payload.CurrentSessionID, payload)
}

func (a *Agent) sessionListPayload(ctx context.Context, includeDeleted bool) (protocol.SessionListResultPayload, error) {
	sessions, err := a.chatService.ListSessionsWithDeleted(ctx, includeDeleted)
	if err != nil {
		return protocol.SessionListResultPayload{}, err
	}
	return protocol.SessionListResultPayload{
		CurrentSessionID: a.chatService.CurrentSessionID(),
		Sessions:         sessionSummaries(sessions),
		IncludeDeleted:   includeDeleted,
	}, nil
}

func (a *Agent) sessionChangedPayload(ctx context.Context) (protocol.SessionChangedPayload, error) {
	list, err := a.sessionListPayload(ctx, false)
	if err != nil {
		return protocol.SessionChangedPayload{}, err
	}

	current := findSessionSummary(list.Sessions, list.CurrentSessionID)
	if current.ID == "" {
		current = protocol.SessionSummary{
			ID:             list.CurrentSessionID,
			Model:          a.chatService.CurrentModelID(),
			AgentMode:      agentModePayload(string(a.chatService.CurrentAgentMode())),
			PermissionMode: string(a.chatService.CurrentPermissionMode()),
			ContextWindowK: a.chatService.CurrentContextWindowK(),
			Usage:          tokenUsagePayloadPtr(a.chatService.CurrentUsage()),
			LastUsage:      tokenUsagePayloadPtr(a.chatService.CurrentLastUsage()),
			CurrentPlan:    planPayload(a.chatService.CurrentPlan()),
		}
	}

	return protocol.SessionChangedPayload{
		CurrentSessionID: list.CurrentSessionID,
		Session:          current,
		Sessions:         list.Sessions,
	}, nil
}
