package agent

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

type Agent struct {
	// Agent 是手机请求在电脑端的传输适配器，业务能力通过窄 Facade 接口注入。
	config            Config
	chatService       ChatFacade
	fileService       WorkspaceFileFacade
	changeService     WorkspaceChangeFacade
	runtimes          *sessionRuntimeManager
	writeMu           sync.Mutex
	requestMu         sync.Mutex
	permissionWaiters *permissionWaiterRegistry
	permissionTimeout time.Duration
}

func New(config Config, chatService ChatFacade, fileService WorkspaceFileFacade, changeService WorkspaceChangeFacade) *Agent {
	if config.BindingCode == "" {
		config.BindingCode = newBindingCode()
	}

	return &Agent{
		config:            config,
		chatService:       chatService,
		fileService:       fileService,
		changeService:     changeService,
		runtimes:          newSessionRuntimeManager(),
		permissionWaiters: newPermissionWaiterRegistry(),
		permissionTimeout: 60 * time.Second,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	if a.config.ServerURL == "" {
		return fmt.Errorf("server url is empty")
	}
	if a.config.UserID == "" {
		return fmt.Errorf("user id is empty")
	}
	if a.config.DeviceID == "" {
		return fmt.Errorf("device id is empty")
	}
	if a.chatService == nil {
		return fmt.Errorf("chat service is nil")
	}
	if a.fileService == nil {
		return fmt.Errorf("file service is nil")
	}
	if a.changeService == nil {
		return fmt.Errorf("change service is nil")
	}
	defer a.changeService.Close()

	fmt.Println("agent starting...")
	fmt.Println("server:", a.config.ServerURL)
	fmt.Println("user:", a.config.UserID)
	fmt.Println("device:", a.config.DeviceID)
	fmt.Println("binding code:", a.config.BindingCode)
	fmt.Println("workspace:", a.fileService.Root())

	// Agent 主动连接 Relay，适合电脑位于 NAT 或内网中的场景。
	conn, response, err := websocket.DefaultDialer.DialContext(ctx, a.config.ServerURL, nil)
	if err != nil {
		if response != nil {
			return fmt.Errorf("connect relay failed: %w, status: %s", err, response.Status)
		}
		return fmt.Errorf("connect relay failed: %w", err)
	}
	defer conn.Close()

	fmt.Println("agent connected.")
	if err := a.writeMessage(conn, protocol.TypeAgentOnline, protocol.AgentOnlinePayload{
		Status:   "online",
		BindCode: a.config.BindingCode,
	}); err != nil {
		return err
	}

	readDone := make(chan error, 1)
	go a.readLoop(ctx, conn, readDone)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = a.writeMessage(conn, protocol.TypeAgentOffline, map[string]string{"status": "offline"})
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "agent stopped"))
			fmt.Println("agent stopped.")
			return nil
		case err := <-readDone:
			return err
		case <-ticker.C:
			if err := a.writeMessage(conn, protocol.TypeHeartbeat, map[string]string{"time": time.Now().Format(time.RFC3339)}); err != nil {
				return err
			}
			fmt.Println("agent heartbeat sent.")
		}
	}
}

func (a *Agent) readLoop(ctx context.Context, conn *websocket.Conn, done chan<- error) {
	for {
		var message protocol.Message
		if err := conn.ReadJSON(&message); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				done <- nil
				return
			}
			done <- fmt.Errorf("read relay message failed: %w", err)
			return
		}

		fmt.Printf("relay message: type=%s request=%s\n", message.Type, message.RequestID)
		if err := a.handleRelayMessage(ctx, conn, message); err != nil {
			if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.ErrorPayload{Message: err.Error()}); writeErr != nil {
				done <- fmt.Errorf("send remote error failed: %w", writeErr)
				return
			}
		}
	}
}

func (a *Agent) writeMessage(conn *websocket.Conn, messageType protocol.MessageType, payload any) error {
	return a.writeRemoteMessage(conn, messageType, newRequestID(), "", payload)
}

func (a *Agent) writeRemoteMessage(conn *websocket.Conn, messageType protocol.MessageType, requestID string, sessionID string, payload any) error {
	message, err := protocol.NewMessage(
		messageType,
		requestID,
		a.config.UserID,
		a.config.DeviceID,
		sessionID,
		payload,
	)
	if err != nil {
		return err
	}

	if err := a.writeJSON(conn, message); err != nil {
		return fmt.Errorf("send %s failed: %w", messageType, err)
	}
	return nil
}

func (a *Agent) handleRelayMessage(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	// 这里只做协议分发；handler 负责 DTO 映射，真正业务继续委托给 ChatFacade。
	switch message.Type {
	case protocol.TypeUserMessage:
		go a.processUserMessage(ctx, conn, message)
	case protocol.TypeSessionRegenerate:
		go a.processRegenerateMessage(ctx, conn, message)
	case protocol.TypePermissionResult:
		return a.handlePermissionResult(message)
	case protocol.TypeSessionList:
		return a.handleSessionList(ctx, conn, message)
	case protocol.TypeSessionNew:
		return a.handleSessionNew(ctx, conn, message)
	case protocol.TypeSessionLoad:
		return a.handleSessionLoad(ctx, conn, message)
	case protocol.TypeSessionDelete:
		return a.handleSessionDelete(ctx, conn, message)
	case protocol.TypeSessionRestore:
		return a.handleSessionRestore(ctx, conn, message)
	case protocol.TypeSessionHistory:
		return a.handleSessionHistory(ctx, conn, message)
	case protocol.TypeSessionHistoryMeta:
		return a.handleSessionHistoryMeta(ctx, conn, message)
	case protocol.TypeSessionHistoryDelta:
		return a.handleSessionHistoryDelta(ctx, conn, message)
	case protocol.TypeSessionPermissionSet:
		return a.handleSessionPermissionSet(ctx, conn, message)
	case protocol.TypeSessionModeSet:
		return a.handleSessionModeSet(ctx, conn, message)
	case protocol.TypeSessionPlanExecute:
		go a.processPlanExecuteMessage(ctx, conn, message)
	case protocol.TypeSessionContextSet:
		return a.handleSessionContextSet(ctx, conn, message)
	case protocol.TypeSessionCompact:
		return a.handleSessionCompact(ctx, conn, message)
	case protocol.TypeSessionPause:
		return a.handleSessionPause(ctx, conn, message)
	case protocol.TypeModelList:
		return a.handleModelList(ctx, conn, message)
	case protocol.TypeModelSwitch:
		return a.handleModelSwitch(ctx, conn, message)
	case protocol.TypeSkillList:
		return a.handleSkillList(ctx, conn, message)
	case protocol.TypeSkillReload:
		return a.handleSkillReload(ctx, conn, message)
	case protocol.TypeAssetList:
		return a.handleAssetList(ctx, conn, message)
	case protocol.TypeFileList:
		return a.handleFileList(ctx, conn, message)
	case protocol.TypeFileRead:
		return a.handleFileRead(ctx, conn, message)
	case protocol.TypeChangesList:
		return a.handleChangesList(ctx, conn, message)
	case protocol.TypeChangeDiff:
		return a.handleChangeDiff(ctx, conn, message)
	case protocol.TypeChangeRevert:
		return a.handleChangeRevert(ctx, conn, message)
	case protocol.TypeHistoryList:
		return a.handleHistoryList(ctx, conn, message)
	case protocol.TypeHistoryDiff:
		return a.handleHistoryDiff(ctx, conn, message)
	case protocol.TypeHistoryRevert:
		return a.handleHistoryRevert(ctx, conn, message)
	default:
		return nil
	}

	return nil
}

func (a *Agent) processUserMessage(ctx context.Context, conn *websocket.Conn, message protocol.Message) {
	sessionID := strings.TrimSpace(message.SessionID)
	if sessionID == "" {
		sessionID = a.chatService.CurrentSessionID()
	}
	// 同一 Session 串行执行，避免两次生成同时追加消息或覆盖 Plan；不同 Session 可并行。
	runtime := a.runtimes.get(sessionID)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runCtx, cancel, ok := runtime.start(ctx)
	if !ok {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, sessionID, protocol.ErrorPayload{Message: "session is already running"}); writeErr != nil {
			log.Printf("send remote busy error failed: %v", writeErr)
		}
		return
	}
	defer runtime.finish(cancel)

	if err := a.handleUserMessage(runCtx, conn, message); err != nil {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.ErrorPayload{Message: err.Error()}); writeErr != nil {
			log.Printf("send remote user message error failed: %v", writeErr)
		}
	}
}

func (a *Agent) processRegenerateMessage(ctx context.Context, conn *websocket.Conn, message protocol.Message) {
	payload, err := protocol.DecodePayload[protocol.SessionRegeneratePayload](message)
	if err != nil {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.ErrorPayload{Message: fmt.Sprintf("decode session regenerate failed: %v", err)}); writeErr != nil {
			log.Printf("send remote regenerate decode error failed: %v", writeErr)
		}
		return
	}

	sessionID := resolveSessionID(payload.SessionID, message.SessionID, a.chatService.CurrentSessionID())
	if sessionID == "" {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.ErrorPayload{Message: "session id is empty"}); writeErr != nil {
			log.Printf("send remote regenerate session error failed: %v", writeErr)
		}
		return
	}

	runtime := a.runtimes.get(sessionID)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runCtx, cancel, ok := runtime.start(ctx)
	if !ok {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, sessionID, protocol.ErrorPayload{Message: "session is already running"}); writeErr != nil {
			log.Printf("send remote busy error failed: %v", writeErr)
		}
		return
	}
	defer runtime.finish(cancel)

	if err := a.handleRegenerateMessage(runCtx, conn, message, sessionID); err != nil {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, sessionID, protocol.ErrorPayload{Message: err.Error()}); writeErr != nil {
			log.Printf("send remote regenerate error failed: %v", writeErr)
		}
	}
}

func (a *Agent) processPlanExecuteMessage(ctx context.Context, conn *websocket.Conn, message protocol.Message) {
	payload, err := protocol.DecodePayload[protocol.SessionPlanExecutePayload](message)
	if err != nil {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.ErrorPayload{Message: fmt.Sprintf("decode session plan execute failed: %v", err)}); writeErr != nil {
			log.Printf("send remote plan execute decode error failed: %v", writeErr)
		}
		return
	}

	sessionID := resolveSessionID(payload.SessionID, message.SessionID, a.chatService.CurrentSessionID())
	if sessionID == "" {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.ErrorPayload{Message: "session id is empty"}); writeErr != nil {
			log.Printf("send remote plan execute session error failed: %v", writeErr)
		}
		return
	}

	// Plan 执行复用同一运行时锁与取消上下文，因此手机“暂停”可以中断当前步骤。
	runtime := a.runtimes.get(sessionID)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runCtx, cancel, ok := runtime.start(ctx)
	if !ok {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, sessionID, protocol.ErrorPayload{Message: "session is already running"}); writeErr != nil {
			log.Printf("send remote busy error failed: %v", writeErr)
		}
		return
	}
	defer runtime.finish(cancel)

	if err := a.handleSessionPlanExecute(runCtx, conn, message, sessionID); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(runCtx.Err(), context.Canceled) {
			if writeErr := a.writePausedAssistantDone(conn, message.RequestID, sessionID); writeErr != nil {
				log.Printf("send remote plan execute paused failed: %v", writeErr)
			}
			return
		}
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, sessionID, protocol.ErrorPayload{Message: err.Error()}); writeErr != nil {
			log.Printf("send remote plan execute error failed: %v", writeErr)
		}
	}
}

func (a *Agent) writeJSON(conn *websocket.Conn, value any) error {
	// reasoning、answer、tool event 可能由不同回调触发，WebSocket 写操作必须串行化。
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	return conn.WriteJSON(value)
}

func newRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func newBindingCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}
