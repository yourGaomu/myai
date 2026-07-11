package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/gorilla/websocket"

	"myai/core/llm"
	agentplan "myai/core/plan"
	"myai/core/remote/protocol"
	"myai/core/service"
)

func (a *Agent) handleUserMessage(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	// Handler 只负责协议校验和流式回包，聊天流程由 ChatService 完成。
	payload, err := protocol.DecodePayload[protocol.UserMessagePayload](message)
	if err != nil {
		return fmt.Errorf("decode user message failed: %w", err)
	}
	if payload.Content == "" {
		return fmt.Errorf("user message content is empty")
	}

	sessionID := strings.TrimSpace(message.SessionID)
	if sessionID == "" {
		sessionID = a.chatService.CurrentSessionID()
	}
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}

	response, err := a.streamChatResponse(ctx, conn, message, sessionID, func(stream llm.ChatStreamHandler) (service.ChatResponse, error) {
		return a.chatService.SendMessageStreamForSession(ctx, sessionID, payload.Content, stream)
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return a.writePausedAssistantDone(conn, message.RequestID, sessionID)
		}
		return err
	}

	return a.writeRemoteMessage(conn, protocol.TypeAssistantDone, message.RequestID, response.SessionID, protocol.AssistantDonePayload{
		Content:   response.Result.Content,
		Reasoning: response.Result.Reasoning,
		Usage:     tokenUsagePayload(response.Result.Usage),
		Context:   contextInfoPayload(response.Context),
		Compact:   compactInfoPayload(response.Compact),
		Plan:      planPayload(response.Plan),
	})
}

func (a *Agent) handleRegenerateMessage(ctx context.Context, conn *websocket.Conn, message protocol.Message, sessionID string) error {
	response, err := a.streamChatResponse(ctx, conn, message, sessionID, func(stream llm.ChatStreamHandler) (service.ChatResponse, error) {
		return a.chatService.RegenerateLastMessageStreamForSession(ctx, sessionID, stream)
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return a.writePausedAssistantDone(conn, message.RequestID, sessionID)
		}
		return err
	}

	return a.writeRemoteMessage(conn, protocol.TypeAssistantDone, message.RequestID, response.SessionID, protocol.AssistantDonePayload{
		Content: response.Result.Content,
		Usage:   tokenUsagePayload(response.Result.Usage),
		Context: contextInfoPayload(response.Context),
		Compact: compactInfoPayload(response.Compact),
		Plan:    planPayload(response.Plan),
	})
}

func (a *Agent) handleSessionPlanExecute(ctx context.Context, conn *websocket.Conn, message protocol.Message, sessionID string) error {
	// 步骤状态变化先发送 plan_update；全部完成后再发送 assistant_done 和最终 result。
	response, err := a.streamChatResponse(ctx, conn, message, sessionID, func(stream llm.ChatStreamHandler) (service.ChatResponse, error) {
		return a.chatService.ExecutePlanStreamForSession(ctx, sessionID, stream, func(currentPlan *agentplan.Plan) {
			payload, err := a.sessionSettingsPayload(ctx, sessionID, service.ContextInfo{}, "")
			if err != nil {
				log.Printf("build plan update failed: %v", err)
				return
			}
			if err := a.writeRemoteMessage(conn, protocol.TypeSessionPlanExecuteUpdate, message.RequestID, sessionID, payload); err != nil {
				log.Printf("send plan update failed: %v", err)
			}
		})
	})
	if err != nil {
		return err
	}

	if err := a.writeRemoteMessage(conn, protocol.TypeAssistantDone, message.RequestID, response.SessionID, protocol.AssistantDonePayload{
		Content:   response.Result.Content,
		Reasoning: response.Result.Reasoning,
		Usage:     tokenUsagePayload(response.Result.Usage),
		Context:   contextInfoPayload(response.Context),
		Compact:   compactInfoPayload(response.Compact),
		Plan:      planPayload(response.Plan),
	}); err != nil {
		return err
	}

	info, err := a.chatService.ContextInfoForSession(ctx, sessionID)
	if err != nil {
		return err
	}
	result, err := a.sessionSettingsPayload(ctx, sessionID, info, "Plan execution finished.")
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionPlanExecuteResult, message.RequestID, sessionID, result)
}

func (a *Agent) streamChatResponse(ctx context.Context, conn *websocket.Conn, message protocol.Message, sessionID string, run func(llm.ChatStreamHandler) (service.ChatResponse, error)) (service.ChatResponse, error) {
	// 把模型回调映射为远程协议事件，手机可分别渲染 reasoning、正文、工具和权限状态。
	sendErrCh := make(chan error, 1)
	send := func(messageType protocol.MessageType, payload any) {
		if err := a.writeRemoteMessage(conn, messageType, message.RequestID, sessionID, payload); err != nil {
			select {
			case sendErrCh <- err:
			default:
			}
		}
	}

	response, err := run(llm.ChatStreamHandler{
		OnReasoning: func(text string) {
			send(protocol.TypeAssistantDelta, protocol.AssistantDeltaPayload{Reasoning: text})
		},
		OnAnswer: func(text string) {
			send(protocol.TypeAssistantDelta, protocol.AssistantDeltaPayload{Content: text})
		},
		OnToolCall: func(name string, arguments string) {
			send(protocol.TypeToolCall, protocol.ToolCallPayload{
				Name:      name,
				Arguments: arguments,
			})
		},
		OnToolResult: func(name string, arguments string, result string) {
			toolFailed := strings.Contains(strings.ToLower(result), "tool error:")
			send(protocol.TypeToolResult, protocol.ToolResultPayload{
				Name:      name,
				Arguments: arguments,
				Result:    result,
				Error:     toolFailed,
			})
			if name != "install_skill" || toolFailed {
				return
			}
			payload, err := a.skillListPayload(ctx, false)
			if err != nil {
				select {
				case sendErrCh <- err:
				default:
				}
				return
			}
			payload.Reloaded = true
			payload.Message = fmt.Sprintf("Reloaded %d local skill(s).", payload.Count)
			send(protocol.TypeSkillReloadResult, payload)
		},
		OnToolAsk: func(request llm.ToolPermissionRequest) bool {
			message.SessionID = sessionID
			return a.askToolPermission(ctx, conn, message, request, sendErrCh)
		},
	})
	if err != nil {
		return service.ChatResponse{}, err
	}

	select {
	case err := <-sendErrCh:
		return service.ChatResponse{}, err
	default:
	}

	return response, nil
}

func (a *Agent) writePausedAssistantDone(conn *websocket.Conn, requestID string, sessionID string) error {
	info, err := a.chatService.ContextInfoForSession(context.Background(), sessionID)
	if err != nil {
		info = service.ContextInfo{}
	}

	return a.writeRemoteMessage(conn, protocol.TypeAssistantDone, requestID, sessionID, protocol.AssistantDonePayload{
		Content: "",
		Context: contextInfoPayload(info),
		Paused:  true,
		Message: "Session task paused.",
	})
}
