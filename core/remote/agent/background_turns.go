package agent

import (
	"context"
	"log"

	"github.com/gorilla/websocket"
	domaininteraction "myai/core/domain/interaction"
	"myai/core/remote/protocol"
	"myai/core/service"
)

type BackgroundTurnSource interface {
	SubscribeBackgroundTurns(buffer int) (<-chan service.BackgroundTurnEvent, func())
}

type BackgroundQuestionSource interface {
	ListBackgroundQuestions(sessionID string) []domaininteraction.Request
}

type BackgroundPermissionControl interface {
	ResolveBackgroundPermission(ctx context.Context, sessionID, turnID, approvalID string, allowed bool) error
}

type BackgroundQuestionControl interface {
	ResolveBackgroundQuestion(ctx context.Context, sessionID, turnID, questionID string, answer domaininteraction.Answer) error
}

type SessionContinuationControl interface {
	PauseSessionContinuations(ctx context.Context, sessionID string) error
}

func backgroundTurnPayload(event service.BackgroundTurnEvent) protocol.BackgroundTurnEventPayload {
	payload := protocol.BackgroundTurnEventPayload{
		TurnID: event.TurnID, RequestID: event.RequestID, SessionID: event.SessionID, RunID: event.RunID, Sequence: event.Sequence,
		Kind: event.Kind, Status: event.Status, Content: event.Content, Reasoning: event.Reasoning, Error: event.Error,
		MessageID: event.MessageID, ApprovalID: event.ApprovalID,
	}
	if event.Permission != nil {
		permission := permissionAskPayload(event.ApprovalID, *event.Permission)
		payload.Permission = &permission
	}
	if event.Question != nil {
		question := userQuestionPayload(*event.Question)
		payload.Question = &question
	}
	if event.Run != nil {
		run := agentRunPayload(*event.Run)
		payload.Run = &run
	}
	if event.Event != nil {
		item := agentRunEventPayload(*event.Event)
		payload.Event = &item
	}
	return payload
}

// handleBackgroundPermissionEvent reports whether a remembered grant resolved
// this ask without a prompt. Otherwise it tracks the server-side reply target.
func (a *Agent) handleBackgroundPermissionEvent(ctx context.Context, event service.BackgroundTurnEvent) bool {
	switch event.Kind {
	case "permission_ask":
		if event.Permission == nil {
			return false
		}
		target := newPermissionRuleTarget(event.SessionID, *event.Permission)
		if a.canReusePermission(target) {
			if control, ok := a.chatService.(BackgroundPermissionControl); ok {
				if control.ResolveBackgroundPermission(ctx, event.SessionID, event.TurnID, event.ApprovalID, true) == nil {
					return true
				}
			}
		}
		a.permissionWaiters.rememberBackground(bgPermissionKey(event.SessionID, event.TurnID, event.ApprovalID), target)
	case "permission_resolved":
		if event.ApprovalID != "" {
			a.permissionWaiters.forgetBackground(bgPermissionKey(event.SessionID, event.TurnID, event.ApprovalID))
		}
	}
	return false
}

func (a *Agent) forwardBackgroundTurns(ctx context.Context, conn *websocket.Conn, events <-chan service.BackgroundTurnEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-events:
			if !open {
				// Overflow retires the connection instead of silently omitting
				// events. Clients resync run/history snapshots after reconnect.
				if ctx.Err() == nil {
					_ = conn.Close()
				}
				return
			}
			if a.handleBackgroundPermissionEvent(ctx, event) {
				continue
			}
			if err := a.writeRemoteMessage(conn, protocol.TypeBackgroundTurnEvent, event.TurnID, event.SessionID, backgroundTurnPayload(event)); err != nil {
				log.Printf("forward background turn failed: %v", err)
				_ = conn.Close()
				return
			}
		}
	}
}
