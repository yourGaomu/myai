package agent

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
	"myai/core/service"
	"myai/core/session"
)

func (a *Agent) sessionSettingsPayload(ctx context.Context, sessionID string, info service.ContextInfo, message string) (protocol.SessionSettingsResultPayload, error) {
	// 设置响应带回服务端最新 Session，手机不能只依赖本地乐观状态。
	list, err := a.sessionListPayload(ctx, false)
	if err != nil {
		return protocol.SessionSettingsResultPayload{}, err
	}

	current := findSessionSummary(list.Sessions, sessionID)
	if current.ID == "" {
		current = protocol.SessionSummary{
			ID:             sessionID,
			Model:          a.chatService.CurrentModelID(),
			AgentMode:      agentModePayload(string(a.chatService.CurrentAgentMode())),
			PermissionMode: string(a.chatService.CurrentPermissionMode()),
			ContextWindowK: a.chatService.CurrentContextWindowK(),
			Usage:          tokenUsagePayloadPtr(a.chatService.CurrentUsage()),
			LastUsage:      tokenUsagePayloadPtr(a.chatService.CurrentLastUsage()),
			CurrentPlan:    planPayload(a.chatService.CurrentPlan()),
		}
	}

	contextPayload := contextInfoPayload(info)
	if current.ID != "" {
		if state, err := a.chatService.ContextStateForSession(ctx, current.ID); err == nil {
			contextPayload = contextStatePayload(state)
		}
	}

	return protocol.SessionSettingsResultPayload{
		CurrentSessionID: list.CurrentSessionID,
		Session:          current,
		Sessions:         list.Sessions,
		Context:          contextPayload,
		Message:          message,
	}, nil
}

func (a *Agent) handleSessionContextQuery(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionContextQueryPayload](message)
	if err != nil {
		return fmt.Errorf("decode session context query failed: %w", err)
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

	state, err := a.chatService.ContextStateForSession(ctx, sessionID)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionContextQueryResult, message.RequestID, sessionID, protocol.SessionContextQueryResultPayload{
		SessionID: sessionID,
		Context:   contextStatePayload(state),
	})
}

func (a *Agent) handleSessionPermissionSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionPermissionSetPayload](message)
	if err != nil {
		return fmt.Errorf("decode session permission set failed: %w", err)
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

	if err := a.chatService.SetPermissionModeForSession(ctx, sessionID, payload.Mode); err != nil {
		return err
	}
	info, err := a.chatService.ContextInfoForSession(ctx, sessionID)
	if err != nil {
		return err
	}
	result, err := a.sessionSettingsPayload(ctx, sessionID, info, fmt.Sprintf("Permission mode set to %s.", payload.Mode))
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionPermissionSetResult, message.RequestID, sessionID, result)
}

func (a *Agent) handleSessionModeSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionModeSetPayload](message)
	if err != nil {
		return fmt.Errorf("decode session mode set failed: %w", err)
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

	// 模式先由应用层校验并持久化，成功后再返回结果驱动手机界面更新。
	if err := a.chatService.SetAgentModeForSession(ctx, sessionID, payload.Mode); err != nil {
		return err
	}
	info, err := a.chatService.ContextInfoForSession(ctx, sessionID)
	if err != nil {
		return err
	}
	result, err := a.sessionSettingsPayload(ctx, sessionID, info, fmt.Sprintf("Mode set to %s.", payload.Mode))
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionModeSetResult, message.RequestID, sessionID, result)
}

func (a *Agent) handleSessionContextSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionContextSetPayload](message)
	if err != nil {
		return fmt.Errorf("decode session context set failed: %w", err)
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

	if err := a.chatService.SetContextWindowKForSession(ctx, sessionID, payload.WindowK); err != nil {
		return err
	}
	info, err := a.chatService.ContextInfoForSession(ctx, sessionID)
	if err != nil {
		return err
	}
	result, err := a.sessionSettingsPayload(ctx, sessionID, info, fmt.Sprintf("Context window set to %dK.", payload.WindowK))
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionContextSetResult, message.RequestID, sessionID, result)
}

func (a *Agent) handleSessionRAGSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionRAGSetPayload](message)
	if err != nil {
		return fmt.Errorf("decode session RAG set failed: %w", err)
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

	settings := session.RAGSettings{
		Mode: session.RetrievalMode(payload.Mode), KnowledgeBaseIDs: payload.KnowledgeBaseIDs,
		CategoryIDs: payload.CategoryIDs, TopK: payload.TopK,
	}
	if err := a.chatService.SetRAGSettingsForSession(ctx, sessionID, settings); err != nil {
		return err
	}
	info, err := a.chatService.ContextInfoForSession(ctx, sessionID)
	if err != nil {
		return err
	}
	result, err := a.sessionSettingsPayload(ctx, sessionID, info, fmt.Sprintf("RAG mode set to %s.", session.NormalizeRAGSettings(settings).Mode))
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionRAGSetResult, message.RequestID, sessionID, result)
}

func (a *Agent) sessionGenerationResult(ctx context.Context, sessionID string, message string) (protocol.SessionGenerationResultPayload, error) {
	preferences, err := a.chatService.SessionPreferencesForSession(ctx, sessionID)
	if err != nil {
		return protocol.SessionGenerationResultPayload{}, err
	}
	return protocol.SessionGenerationResultPayload{
		Preferences: sessionGenerationPreferencesPayload(sessionID, preferences),
		Message:     message,
	}, nil
}

func (a *Agent) handleSessionGenerationQuery(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionGenerationQueryPayload](message)
	if err != nil {
		return fmt.Errorf("decode session generation query failed: %w", err)
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

	result, err := a.sessionGenerationResult(ctx, sessionID, "")
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionGenerationQueryResult, message.RequestID, sessionID, result)
}

func (a *Agent) handleSessionGenerationSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionGenerationSetPayload](message)
	if err != nil {
		return fmt.Errorf("decode session generation set failed: %w", err)
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

	if err := a.chatService.SetGenerationSettingsForSession(ctx, sessionID, generationSettingsFromPayload(payload.Settings)); err != nil {
		return err
	}
	result, err := a.sessionGenerationResult(ctx, sessionID, "生成参数已保存。")
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionGenerationSetResult, message.RequestID, sessionID, result)
}

func (a *Agent) handleSessionStyleSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionStyleSetPayload](message)
	if err != nil {
		return fmt.Errorf("decode session style set failed: %w", err)
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

	if err := a.chatService.SetStyleInstructionForSession(ctx, sessionID, payload.StyleInstruction); err != nil {
		return err
	}
	result, err := a.sessionGenerationResult(ctx, sessionID, "回复风格已保存。")
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionStyleSetResult, message.RequestID, sessionID, result)
}

func (a *Agent) handleSessionCompact(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionCompactPayload](message)
	if err != nil {
		return fmt.Errorf("decode session compact failed: %w", err)
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

	info, err := a.chatService.CompactSession(ctx, sessionID)
	if err != nil {
		return err
	}
	result, err := a.sessionSettingsPayload(ctx, sessionID, info, "Context compacted.")
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionCompactResult, message.RequestID, sessionID, result)
}

func (a *Agent) handleSessionPause(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SessionPausePayload](message)
	if err != nil {
		return fmt.Errorf("decode session pause failed: %w", err)
	}
	sessionID := resolveSessionID(payload.SessionID, message.SessionID, a.chatService.CurrentSessionID())
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}

	paused := a.runtimes.get(sessionID).pause()
	text := "No running task for this session."
	if paused {
		text = "Session task paused."
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionPauseResult, message.RequestID, sessionID, protocol.SessionPauseResultPayload{
		SessionID: sessionID,
		Paused:    paused,
		Message:   text,
	})
}

func (a *Agent) handleModelSwitch(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ModelSwitchPayload](message)
	if err != nil {
		return fmt.Errorf("decode model switch failed: %w", err)
	}
	sessionID := resolveSessionID("", message.SessionID, a.chatService.CurrentSessionID())
	runtime := a.runtimes.get(sessionID)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	a.requestMu.Lock()
	defer a.requestMu.Unlock()

	if err := a.chatService.SwitchModelForSession(ctx, sessionID, payload.ModelID); err != nil {
		return err
	}

	info, _ := a.chatService.ContextInfoForSession(ctx, sessionID)
	sessionPayload, err := a.sessionSettingsPayload(ctx, sessionID, info, fmt.Sprintf("Switched model to %s.", payload.ModelID))
	if err != nil {
		return err
	}
	result := protocol.ModelSwitchResultPayload{
		CurrentModelID: payload.ModelID,
		Models:         modelSummaries(a.chatService.ListModels()),
		Session:        sessionPayload.Session,
		Message:        sessionPayload.Message,
	}
	return a.writeRemoteMessage(conn, protocol.TypeModelSwitchResult, message.RequestID, sessionID, result)
}
