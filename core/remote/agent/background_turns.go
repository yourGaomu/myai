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
		ApprovalID: event.ApprovalID,
	}
	if event.Permission != nil {
		payload.Permission = &protocol.PermissionAskPayload{ApprovalID: event.ApprovalID, Name: event.Permission.Name, Arguments: event.Permission.Arguments, Permission: string(event.Permission.Permission)}
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
			if err := a.writeRemoteMessage(conn, protocol.TypeBackgroundTurnEvent, event.TurnID, event.SessionID, backgroundTurnPayload(event)); err != nil {
				log.Printf("forward background turn failed: %v", err)
				_ = conn.Close()
				return
			}
		}
	}
}
