package agent

import (
	"context"
	"log"

	"github.com/gorilla/websocket"
	"myai/core/remote/protocol"
	"myai/core/service"
)

type BackgroundTurnSource interface {
	SubscribeBackgroundTurns(buffer int) (<-chan service.BackgroundTurnEvent, func())
}

type SessionContinuationControl interface {
	PauseSessionContinuations(ctx context.Context, sessionID string) error
}

func backgroundTurnPayload(event service.BackgroundTurnEvent) protocol.BackgroundTurnEventPayload {
	payload := protocol.BackgroundTurnEventPayload{
		TurnID: event.TurnID, SessionID: event.SessionID, RunID: event.RunID, Sequence: event.Sequence,
		Kind: event.Kind, Status: event.Status, Content: event.Content, Reasoning: event.Reasoning, Error: event.Error,
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
