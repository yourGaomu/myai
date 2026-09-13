package agent

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
	"myai/core/remote/protocol"
)

type Agent struct {
	// Agent 是手机请求在电脑端的传输适配器，业务能力通过窄 Facade 接口注入。
	config            Config
	chatService       ChatFacade
	fileService       WorkspaceFileFacade
	changeService     WorkspaceChangeFacade
	knowledgeService  KnowledgeFacade
	memoryService     MemoryFacade
	memoryExtraction  MemoryExtractionFacade
	memoryDream       MemoryDreamFacade
	subagentService   SubagentFacade
	subagentEvents    SubagentEventSource
	pluginManager     PluginManagerFacade
	runtimes          *sessionRuntimeManager
	writeMu           sync.Mutex
	requestMu         sync.Mutex
	permissionWaiters *permissionWaiterRegistry
	permissionTimeout time.Duration
}

func New(config Config, chatService ChatFacade, fileService WorkspaceFileFacade, changeService WorkspaceChangeFacade, knowledgeService KnowledgeFacade, memoryService MemoryFacade, memoryExtraction MemoryExtractionFacade, memoryDream MemoryDreamFacade, subagentService SubagentFacade, subagentEvents SubagentEventSource, pluginManager PluginManagerFacade) *Agent {
	if config.BindingCode == "" {
		config.BindingCode = newBindingCode()
	}

	return &Agent{
		config:            config,
		chatService:       chatService,
		fileService:       fileService,
		changeService:     changeService,
		knowledgeService:  knowledgeService,
		memoryService:     memoryService,
		memoryExtraction:  memoryExtraction,
		memoryDream:       memoryDream,
		subagentService:   subagentService,
		subagentEvents:    subagentEvents,
		pluginManager:     pluginManager,
		runtimes:          newSessionRuntimeManager(),
		permissionWaiters: newPermissionWaiterRegistry(),
		permissionTimeout: 60 * time.Second,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	if a.config.ServerURL == "" {
		return fmt.Errorf("server url is empty")
	}
	if strings.TrimSpace(a.config.RelayToken) == "" {
		return fmt.Errorf("relay agent token is empty")
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

	backoff := time.Second
	for {
		connected, err := a.runConnection(ctx)
		if ctx.Err() != nil {
			fmt.Println("agent stopped.")
			return nil
		}
		if err != nil {
			log.Printf("agent connection ended: %v; reconnecting in %s", err, backoff)
		}
		if connected {
			backoff = time.Second
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			fmt.Println("agent stopped.")
			return nil
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

// runConnection owns one websocket lifetime. Keeping it separate makes a
// dropped relay connection recoverable without losing the process and its
// background subagent scheduler.
func (a *Agent) runConnection(ctx context.Context) (bool, error) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+strings.TrimSpace(a.config.RelayToken))
	headers.Set(protocol.HeaderAgentUserID, strings.TrimSpace(a.config.UserID))
	headers.Set(protocol.HeaderAgentDeviceID, strings.TrimSpace(a.config.DeviceID))
	conn, response, err := websocket.DefaultDialer.DialContext(ctx, a.config.ServerURL, headers)
	if err != nil {
		if response != nil {
			return false, fmt.Errorf("connect relay failed: %w, status: %s", err, response.Status)
		}
		return false, fmt.Errorf("connect relay failed: %w", err)
	}
	defer conn.Close()

	fmt.Println("agent connected.")
	if err := a.writeMessage(conn, protocol.TypeAgentOnline, protocol.AgentOnlinePayload{
		Status: "online", BindCode: a.config.BindingCode,
	}); err != nil {
		return true, err
	}
	if a.subagentEvents != nil {
		if source, ok := a.subagentEvents.(SubagentTaskEventSource); ok {
			events, unsubscribe := source.SubscribeTaskEvents("", 0, 32)
			defer unsubscribe()
			go a.forwardSubagentTaskEvents(ctx, conn, events)
		} else {
			events, unsubscribe := a.subagentEvents.Subscribe(32)
			defer unsubscribe()
			go a.forwardSubagentEvents(ctx, conn, events)
		}
	}

	readDone := make(chan error, 1)
	go a.readLoop(ctx, conn, readDone)
	// Keep an application-level heartbeat comfortably below common reverse proxy
	// idle timeouts. Relay also sends protocol-level ping frames, but this
	// heartbeat updates the agent registry's last-seen timestamp.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = a.writeMessage(conn, protocol.TypeAgentOffline, map[string]string{"status": "offline"})
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "agent stopped"))
			return true, nil
		case err := <-readDone:
			return true, err
		case <-ticker.C:
			if err := a.writeMessage(conn, protocol.TypeHeartbeat, map[string]string{"time": time.Now().Format(time.RFC3339)}); err != nil {
				return true, err
			}
			fmt.Println("agent heartbeat sent.")
		}
	}
}

func (a *Agent) forwardSubagentEvents(ctx context.Context, conn *websocket.Conn, events <-chan domainsubagent.Task) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-events:
			if !ok {
				return
			}
			if err := a.writeRemoteMessage(conn, protocol.TypeSubagentTaskEvent, newRequestID(), task.ParentSessionID, protocol.SubagentTaskResultPayload{
				Task: subagentTaskPayload(task),
			}); err != nil {
				log.Printf("send subagent task event failed: %v", err)
				return
			}
		}
	}
}

func (a *Agent) forwardSubagentTaskEvents(ctx context.Context, conn *websocket.Conn, events <-chan subagentport.TaskEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := a.writeRemoteMessage(conn, protocol.TypeSubagentTaskEvent, newRequestID(), event.Task.ParentSessionID, protocol.SubagentTaskResultPayload{
				Task: subagentTaskPayload(event.Task), Sequence: event.Sequence, Kind: event.Kind, EmittedAt: event.EmittedAt,
				RunID: event.RunID, Content: event.Content, ToolName: event.ToolName, Arguments: event.Arguments,
				Status: event.Status, ErrorCode: event.ErrorCode, ErrorMessage: event.ErrorMessage,
				Truncated: event.Truncated, Delta: event.Delta,
			}); err != nil {
				log.Printf("send subagent task event failed: %v", err)
				return
			}
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
	case protocol.TypeAgentRunList:
		return a.handleAgentRunList(ctx, conn, message)
	case protocol.TypeSessionPermissionSet:
		return a.handleSessionPermissionSet(ctx, conn, message)
	case protocol.TypeSessionModeSet:
		return a.handleSessionModeSet(ctx, conn, message)
	case protocol.TypeSessionPlanExecute:
		go a.processPlanExecuteMessage(ctx, conn, message)
	case protocol.TypeSessionContextQuery:
		return a.handleSessionContextQuery(ctx, conn, message)
	case protocol.TypeSessionContextSet:
		return a.handleSessionContextSet(ctx, conn, message)
	case protocol.TypeSessionRAGSet:
		return a.handleSessionRAGSet(ctx, conn, message)
	case protocol.TypeSessionGenerationQuery:
		return a.handleSessionGenerationQuery(ctx, conn, message)
	case protocol.TypeSessionGenerationSet:
		return a.handleSessionGenerationSet(ctx, conn, message)
	case protocol.TypeSessionStyleSet:
		return a.handleSessionStyleSet(ctx, conn, message)
	case protocol.TypeSessionCompact:
		return a.handleSessionCompact(ctx, conn, message)
	case protocol.TypeSessionPause:
		return a.handleSessionPause(ctx, conn, message)
	case protocol.TypeModelList:
		return a.handleModelList(ctx, conn, message)
	case protocol.TypeModelSwitch:
		return a.handleModelSwitch(ctx, conn, message)
	case protocol.TypeModelConfigAdd:
		return a.handleModelConfigAdd(ctx, conn, message)
	case protocol.TypeModelConfigTest:
		return a.handleModelConfigTest(ctx, conn, message)
	case protocol.TypeModelConfigUpdate:
		return a.handleModelConfigUpdate(ctx, conn, message)
	case protocol.TypeModelConfigDelete:
		return a.handleModelConfigDelete(ctx, conn, message)
	case protocol.TypeModelConfigEnabledSet:
		return a.handleModelConfigEnabledSet(ctx, conn, message)
	case protocol.TypeModelConfigDefaultSet:
		return a.handleModelConfigDefaultSet(ctx, conn, message)
	case protocol.TypeSkillList:
		return a.handleSkillList(ctx, conn, message)
	case protocol.TypeSkillReload:
		return a.handleSkillReload(ctx, conn, message)
	case protocol.TypePluginList:
		return a.handlePluginList(ctx, conn, message)
	case protocol.TypePluginReload:
		return a.handlePluginReload(ctx, conn, message)
	case protocol.TypePluginEnable:
		return a.handlePluginToggle(ctx, conn, message, true)
	case protocol.TypePluginDisable:
		return a.handlePluginToggle(ctx, conn, message, false)
	case protocol.TypeAssetList:
		return a.handleAssetList(ctx, conn, message)
	case protocol.TypeKnowledgeCatalogList:
		return a.handleKnowledgeCatalogList(ctx, conn, message)
	case protocol.TypeKnowledgeCategoryCreate:
		return a.handleKnowledgeCategoryCreate(ctx, conn, message)
	case protocol.TypeKnowledgeCategoryMove:
		return a.handleKnowledgeCategoryMove(ctx, conn, message)
	case protocol.TypeKnowledgeCategoryDelete:
		return a.handleKnowledgeCategoryDelete(ctx, conn, message)
	case protocol.TypeKnowledgeBaseCreate:
		return a.handleKnowledgeBaseCreate(ctx, conn, message)
	case protocol.TypeKnowledgeBaseUpdate:
		return a.handleKnowledgeBaseUpdate(ctx, conn, message)
	case protocol.TypeKnowledgeBaseDelete:
		return a.handleKnowledgeBaseDelete(ctx, conn, message)
	case protocol.TypeKnowledgeDocumentList:
		return a.handleKnowledgeDocumentList(ctx, conn, message)
	case protocol.TypeKnowledgeDocumentIngest:
		return a.handleKnowledgeDocumentIngest(ctx, conn, message)
	case protocol.TypeKnowledgeDocumentRetry:
		return a.handleKnowledgeDocumentRetry(ctx, conn, message)
	case protocol.TypeKnowledgeDocumentDelete:
		return a.handleKnowledgeDocumentDelete(ctx, conn, message)
	case protocol.TypeKnowledgeProfileList:
		return a.handleKnowledgeProfileList(ctx, conn, message)
	case protocol.TypeKnowledgeSearchPreview:
		return a.handleKnowledgeSearchPreview(ctx, conn, message)
	case protocol.TypeAIMemoryList:
		return a.handleAIMemoryList(ctx, conn, message)
	case protocol.TypeAIMemoryCreate:
		return a.handleAIMemoryCreate(ctx, conn, message)
	case protocol.TypeAIMemoryUpdate:
		return a.handleAIMemoryUpdate(ctx, conn, message)
	case protocol.TypeAIMemoryDelete:
		return a.handleAIMemoryDelete(ctx, conn, message)
	case protocol.TypeAIMemoryRestore:
		return a.handleAIMemoryRestore(ctx, conn, message)
	case protocol.TypeAIMemoryCandidateList:
		return a.handleAIMemoryCandidateList(ctx, conn, message)
	case protocol.TypeAIMemoryCandidateApprove:
		return a.handleAIMemoryCandidateApprove(ctx, conn, message)
	case protocol.TypeAIMemoryCandidateReject:
		return a.handleAIMemoryCandidateReject(ctx, conn, message)
	case protocol.TypeAIMemoryExtractionJobList:
		return a.handleAIMemoryExtractionJobList(ctx, conn, message)
	case protocol.TypeAIMemoryExtractionJobRetry:
		return a.handleAIMemoryExtractionJobRetry(ctx, conn, message)
	case protocol.TypeAIMemoryDreamRun:
		go a.processAIMemoryDreamRun(ctx, conn, message)
	case protocol.TypeAIMemoryDreamList:
		return a.handleAIMemoryDreamList(ctx, conn, message)
	case protocol.TypeSubagentDefinitionList:
		return a.handleSubagentDefinitionList(ctx, conn, message)
	case protocol.TypeSubagentDefinitionCreate:
		return a.handleSubagentDefinitionCreate(ctx, conn, message)
	case protocol.TypeSubagentDefinitionUpdate:
		return a.handleSubagentDefinitionUpdate(ctx, conn, message)
	case protocol.TypeSubagentDefinitionDelete:
		return a.handleSubagentDefinitionDelete(ctx, conn, message)
	case protocol.TypeSubagentTaskList:
		return a.handleSubagentTaskList(ctx, conn, message)
	case protocol.TypeSubagentTaskCheck:
		return a.handleSubagentTaskCheck(ctx, conn, message)
	case protocol.TypeSubagentTaskMessage:
		return a.handleSubagentTaskMessage(ctx, conn, message)
	case protocol.TypeSubagentTaskFollowup:
		return a.handleSubagentTaskFollowup(ctx, conn, message)
	case protocol.TypeSubagentTaskWait:
		return a.handleSubagentTaskWait(ctx, conn, message)
	case protocol.TypeSubagentTaskCancel:
		return a.handleSubagentTaskCancel(ctx, conn, message)
	case protocol.TypeSubagentTaskApply:
		return a.handleSubagentTaskApply(ctx, conn, message)
	case protocol.TypeSubagentTaskDiscard:
		return a.handleSubagentTaskDiscard(ctx, conn, message)
	case protocol.TypeSubagentTaskResume:
		go a.processSubagentTaskResume(ctx, conn, message)
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

func (a *Agent) processAIMemoryDreamRun(ctx context.Context, conn *websocket.Conn, message protocol.Message) {
	if err := a.handleAIMemoryDreamRun(ctx, conn, message); err != nil {
		if writeErr := a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.ErrorPayload{Message: err.Error()}); writeErr != nil {
			log.Printf("send AI memory dream error failed: %v", writeErr)
		}
	}
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
			if writeErr := a.writeCanceledPlanExecution(conn, message.RequestID, sessionID); writeErr != nil {
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
