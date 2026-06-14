package agent

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/llm"
	"myai/core/remote/changes"
	"myai/core/remote/files"
	"myai/core/remote/protocol"
	"myai/core/service"
	"myai/core/store/data"
)

type Config struct {
	ServerURL   string
	UserID      string
	DeviceID    string
	BindingCode string
	Workspace   string
}

type Agent struct {
	config            Config
	chatService       *service.ChatService
	fileService       *files.Service
	changeService     *changes.Service
	writeMu           sync.Mutex
	requestMu         sync.Mutex
	permissionMu      sync.Mutex
	permissions       map[string]chan bool
	permissionTimeout time.Duration
	fileServiceErr    error
	changeServiceErr  error
}

func New(config Config, chatService *service.ChatService) *Agent {
	if config.BindingCode == "" {
		config.BindingCode = newBindingCode()
	}
	fileService, err := files.New(config.Workspace)
	changeService, changeErr := changes.New(config.Workspace)

	return &Agent{
		config:            config,
		chatService:       chatService,
		fileService:       fileService,
		changeService:     changeService,
		permissions:       make(map[string]chan bool),
		permissionTimeout: 60 * time.Second,
		fileServiceErr:    err,
		changeServiceErr:  changeErr,
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
	if a.fileServiceErr != nil {
		return fmt.Errorf("file workspace is invalid: %w", a.fileServiceErr)
	}
	if a.changeServiceErr != nil {
		return fmt.Errorf("change workspace is invalid: %w", a.changeServiceErr)
	}
	defer a.changeService.Close()

	fmt.Println("agent starting...")
	fmt.Println("server:", a.config.ServerURL)
	fmt.Println("user:", a.config.UserID)
	fmt.Println("device:", a.config.DeviceID)
	fmt.Println("binding code:", a.config.BindingCode)
	fmt.Println("workspace:", a.fileService.Root())

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
	switch message.Type {
	case protocol.TypeUserMessage:
		go a.processUserMessage(ctx, conn, message)
	case protocol.TypePermissionResult:
		return a.handlePermissionResult(message)
	case protocol.TypeSessionList:
		return a.handleSessionList(ctx, conn, message)
	case protocol.TypeSessionNew:
		return a.handleSessionNew(ctx, conn, message)
	case protocol.TypeSessionLoad:
		return a.handleSessionLoad(ctx, conn, message)
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
	a.requestMu.Lock()
	defer a.requestMu.Unlock()

	if err := a.handleUserMessage(ctx, conn, message); err != nil {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.ErrorPayload{Message: err.Error()}); writeErr != nil {
			log.Printf("send remote user message error failed: %v", writeErr)
		}
	}
}

func (a *Agent) handlePermissionResult(message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.PermissionResultPayload](message)
	if err != nil {
		return fmt.Errorf("decode permission result failed: %w", err)
	}

	a.permissionMu.Lock()
	ch := a.permissions[message.RequestID]
	a.permissionMu.Unlock()
	if ch == nil {
		return nil
	}

	select {
	case ch <- payload.Allowed:
	default:
	}
	return nil
}

func (a *Agent) handleSessionList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	a.requestMu.Lock()
	defer a.requestMu.Unlock()

	payload, err := a.sessionListPayload(ctx)
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

func (a *Agent) handleFileList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.FileListPayload](message)
	if err != nil {
		return fmt.Errorf("decode file list failed: %w", err)
	}

	result, err := a.fileService.List(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeFileListResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleFileRead(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.FileReadPayload](message)
	if err != nil {
		return fmt.Errorf("decode file read failed: %w", err)
	}

	result, err := a.fileService.Read(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeFileReadResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleChangesList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ChangesListPayload](message)
	if err != nil {
		return fmt.Errorf("decode changes list failed: %w", err)
	}

	result, err := a.changeService.List(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeChangesListResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleChangeDiff(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ChangeDiffPayload](message)
	if err != nil {
		return fmt.Errorf("decode change diff failed: %w", err)
	}

	result, err := a.changeService.Diff(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeChangeDiffResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleChangeRevert(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ChangeRevertPayload](message)
	if err != nil {
		return fmt.Errorf("decode change revert failed: %w", err)
	}

	result, err := a.changeService.Revert(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeChangeRevertResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleHistoryList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.HistoryListPayload](message)
	if err != nil {
		return fmt.Errorf("decode history list failed: %w", err)
	}

	result, err := a.changeService.History(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeHistoryListResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleHistoryDiff(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.HistoryDiffPayload](message)
	if err != nil {
		return fmt.Errorf("decode history diff failed: %w", err)
	}

	result, err := a.changeService.HistoryDiff(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeHistoryDiffResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleHistoryRevert(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.HistoryRevertPayload](message)
	if err != nil {
		return fmt.Errorf("decode history revert failed: %w", err)
	}

	result, err := a.changeService.RevertCheckpoint(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeHistoryRevertResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) writeSessionChanged(ctx context.Context, conn *websocket.Conn, requestID string) error {
	payload, err := a.sessionChangedPayload(ctx)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSessionChanged, requestID, payload.CurrentSessionID, payload)
}

func (a *Agent) sessionListPayload(ctx context.Context) (protocol.SessionListResultPayload, error) {
	sessions, err := a.chatService.ListSessions(ctx)
	if err != nil {
		return protocol.SessionListResultPayload{}, err
	}
	return protocol.SessionListResultPayload{
		CurrentSessionID: a.chatService.CurrentSessionID(),
		Sessions:         sessionSummaries(sessions),
	}, nil
}

func (a *Agent) sessionChangedPayload(ctx context.Context) (protocol.SessionChangedPayload, error) {
	list, err := a.sessionListPayload(ctx)
	if err != nil {
		return protocol.SessionChangedPayload{}, err
	}

	current := findSessionSummary(list.Sessions, list.CurrentSessionID)
	if current.ID == "" {
		current = protocol.SessionSummary{
			ID:             list.CurrentSessionID,
			Model:          a.chatService.CurrentModelID(),
			PermissionMode: string(a.chatService.CurrentPermissionMode()),
			ContextWindowK: a.chatService.CurrentContextWindowK(),
		}
	}

	return protocol.SessionChangedPayload{
		CurrentSessionID: list.CurrentSessionID,
		Session:          current,
		Sessions:         list.Sessions,
	}, nil
}

func (a *Agent) handleUserMessage(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.UserMessagePayload](message)
	if err != nil {
		return fmt.Errorf("decode user message failed: %w", err)
	}
	if payload.Content == "" {
		return fmt.Errorf("user message content is empty")
	}

	sessionID := message.SessionID
	if sessionID != "" && sessionID != a.chatService.CurrentSessionID() {
		if err := a.chatService.LoadSession(ctx, sessionID); err != nil {
			return err
		}
	}
	if sessionID == "" {
		sessionID = a.chatService.CurrentSessionID()
	}

	sendErrCh := make(chan error, 1)
	send := func(messageType protocol.MessageType, payload any) {
		if err := a.writeRemoteMessage(conn, messageType, message.RequestID, sessionID, payload); err != nil {
			select {
			case sendErrCh <- err:
			default:
			}
		}
	}

	response, err := a.chatService.SendMessageStream(ctx, payload.Content, llm.ChatStreamHandler{
		OnAnswer: func(text string) {
			send(protocol.TypeAssistantDelta, protocol.AssistantDeltaPayload{Content: text})
		},
		OnToolCall: func(name string, arguments string) {
			send(protocol.TypeToolCall, protocol.ToolCallPayload{
				Name:      name,
				Arguments: arguments,
			})
		},
		OnToolAsk: func(request llm.ToolPermissionRequest) bool {
			message.SessionID = sessionID
			return a.askToolPermission(ctx, conn, message, request, sendErrCh)
		},
	})
	if err != nil {
		return err
	}

	select {
	case err := <-sendErrCh:
		return err
	default:
	}

	return a.writeRemoteMessage(conn, protocol.TypeAssistantDone, message.RequestID, response.SessionID, protocol.AssistantDonePayload{Content: response.Result.Content})
}

func sessionSummaries(sessions []data.SessionRecord) []protocol.SessionSummary {
	summaries := make([]protocol.SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		summaries = append(summaries, protocol.SessionSummary{
			ID:             session.ID,
			Title:          session.Title,
			Model:          session.Model,
			PermissionMode: session.PermissionMode,
			ContextWindowK: session.ContextWindowK,
			CreatedAt:      session.CreatedAt,
			UpdatedAt:      session.UpdatedAt,
		})
	}
	return summaries
}

func findSessionSummary(sessions []protocol.SessionSummary, sessionID string) protocol.SessionSummary {
	for _, session := range sessions {
		if session.ID == sessionID {
			return session
		}
	}
	return protocol.SessionSummary{}
}

func (a *Agent) askToolPermission(ctx context.Context, conn *websocket.Conn, message protocol.Message, request llm.ToolPermissionRequest, sendErrCh chan<- error) bool {
	if message.RequestID == "" {
		return false
	}

	ch := a.registerPermissionWaiter(message.RequestID)
	defer a.unregisterPermissionWaiter(message.RequestID, ch)

	if err := a.writeRemoteMessage(conn, protocol.TypePermissionAsk, message.RequestID, message.SessionID, protocol.PermissionAskPayload{
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

func (a *Agent) registerPermissionWaiter(requestID string) chan bool {
	ch := make(chan bool, 1)

	a.permissionMu.Lock()
	defer a.permissionMu.Unlock()

	a.permissions[requestID] = ch
	return ch
}

func (a *Agent) unregisterPermissionWaiter(requestID string, ch chan bool) {
	a.permissionMu.Lock()
	defer a.permissionMu.Unlock()

	if a.permissions[requestID] == ch {
		delete(a.permissions, requestID)
	}
}

func (a *Agent) writeJSON(conn *websocket.Conn, value any) error {
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
