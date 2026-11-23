package agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	subagentcommand "myai/core/application/subagent/command"
	domainsubagent "myai/core/domain/subagent"
	"myai/core/port/files"
	subagentport "myai/core/port/subagent"
	"myai/core/remote/protocol"
	"myai/core/remote/uploads"
)

type Agent struct {
	// Agent 是手机请求在电脑端的传输适配器，业务能力通过窄 Facade 接口注入。
	config                Config
	chatService           ChatFacade
	fileService           WorkspaceFileFacade
	changeService         WorkspaceChangeFacade
	knowledgeService      KnowledgeFacade
	memoryService         MemoryFacade
	memoryExtraction      MemoryExtractionFacade
	memoryDream           MemoryDreamFacade
	subagentService       SubagentFacade
	subagentEvents        SubagentEventSource
	pluginManager         PluginManagerFacade
	mcpManager            MCPManagerFacade
	runtimes              *sessionRuntimeManager
	writeMu               sync.Mutex
	requestMu             sync.Mutex
	catalogMu             sync.Mutex
	operations            operationQueue
	catalogOperations     operationQueue
	activeOperations      atomic.Int32
	tasks                 sync.WaitGroup
	lastTaskEventSequence atomic.Uint64
	permissionWaiters     *permissionWaiterRegistry
	permissionTimeout     time.Duration
	userQuestionWaiters   *userQuestionWaiterRegistry
	userQuestionTimeout   time.Duration
	activeSubagentWaits   atomic.Int32
	uploadService         files.Service
	fileUploadAdmission   func(context.Context, string) (uint64, error)
	fileAssetRecorder     uploads.Recorder
	uploadMu              sync.Mutex
	uploadReceiver        *uploads.Receiver
	uploadsClosed         bool
	uploadOperations      operationQueue
	activeUploadReplies   atomic.Int32
	activeImagePreviews   atomic.Int32
}

// WithMCPManager attaches the narrow application-level MCP reload facade.
// Keeping it optional preserves agents that only expose chat and plugins.
func (a *Agent) WithMCPManager(manager MCPManagerFacade) *Agent {
	if a != nil {
		a.mcpManager = manager
	}
	return a
}

func New(config Config, chatService ChatFacade, fileService WorkspaceFileFacade, changeService WorkspaceChangeFacade, knowledgeService KnowledgeFacade, memoryService MemoryFacade, memoryExtraction MemoryExtractionFacade, memoryDream MemoryDreamFacade, subagentService SubagentFacade, subagentEvents SubagentEventSource, pluginManager PluginManagerFacade, options ...Option) *Agent {
	if config.BindingCode == "" {
		config.BindingCode = newBindingCode()
	}

	a := &Agent{
		config:              config,
		chatService:         chatService,
		fileService:         fileService,
		changeService:       changeService,
		knowledgeService:    knowledgeService,
		memoryService:       memoryService,
		memoryExtraction:    memoryExtraction,
		memoryDream:         memoryDream,
		subagentService:     subagentService,
		subagentEvents:      subagentEvents,
		pluginManager:       pluginManager,
		runtimes:            newSessionRuntimeManager(),
		permissionWaiters:   newPermissionWaiterRegistry(),
		permissionTimeout:   60 * time.Second,
		userQuestionWaiters: newUserQuestionWaiterRegistry(),
		userQuestionTimeout: 15 * time.Minute,
	}
	for _, option := range options {
		if option != nil {
			option(a)
		}
	}
	return a
}

func (a *Agent) Run(ctx context.Context) (runErr error) {
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
	ctx, cancelRun := context.WithCancel(ctx)
	defer func() {
		cancelRun()
		if err := a.closeFileUploads(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close file uploads: %w", err))
		}
		joinCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.waitForOperations(joinCtx); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("wait for remote operations: %w", err))
		}
	}()

	fmt.Println("agent starting...")
	fmt.Println("server:", a.config.ServerURL)
	fmt.Println("user:", a.config.UserID)
	fmt.Println("device:", a.config.DeviceID)
	fmt.Println("binding code:", a.config.BindingCode)
	fmt.Println("workspace:", a.fileService.Root())

	backoff := time.Second
	for {
		//循环调用
		connectedAt := time.Now()
		connected, err := a.runConnection(ctx)
		if ctx.Err() != nil {
			fmt.Println("agent stopped.")
			return nil
		}
		if permanentRelayError(err) {
			return err
		}
		if connected && time.Since(connectedAt) >= 30*time.Second {
			backoff = time.Second
		}
		delay := reconnectDelay(backoff)
		if err != nil {
			log.Printf("agent connection ended: %v; reconnecting in %s", err, delay)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			fmt.Println("agent stopped.")
			return nil
		case <-timer.C:
		}
		backoff = nextReconnectBackoff(backoff)
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
		return false, relayDialError(err, response)
	}
	defer conn.Close()
	conn.SetReadLimit(8 * 1024 * 1024)

	fmt.Println("agent connected.")
	if err := a.writeMessage(conn, protocol.TypeAgentOnline, protocol.AgentOnlinePayload{
		Status: "online", BindCode: a.config.BindingCode,
		LastTaskEventSequence: a.lastTaskEventSequence.Load(),
	}); err != nil {
		return true, err
	}
	eventCtx, cancelEvents := context.WithCancel(ctx)
	defer cancelEvents()
	if source, ok := a.chatService.(BackgroundTurnSource); ok {
		events, unsubscribe := source.SubscribeBackgroundTurns(256)
		defer unsubscribe()
		// Clients may remain connected to Relay while this Agent reconnects.
		// Signal snapshot recovery for any events missed during the outage.
		if err := a.writeRemoteMessage(conn, protocol.TypeBackgroundTurnEvent, newRequestID(), "", protocol.BackgroundTurnEventPayload{Kind: "resync_required"}); err != nil {
			return true, err
		}
		go a.forwardBackgroundTurns(eventCtx, conn, events)
	}
	//是否存在子智能体调用能力
	if a.subagentEvents != nil {
		if source, ok := a.subagentEvents.(SubagentTaskEventSource); ok {
			go a.forwardSubagentTaskEvents(eventCtx, conn, source)
		} else {
			events, unsubscribe := a.subagentEvents.Subscribe(32)
			defer unsubscribe()
			go a.forwardSubagentEvents(eventCtx, conn, events)
		}
	}

	readDone := make(chan error, 1)
	connectionCtx, cancelConnection := context.WithCancel(ctx)
	a.tasks.Add(1)
	go func() {
		defer a.tasks.Done()
		a.readLoopWithConnectionContext(ctx, connectionCtx, conn, readDone)
	}()
	defer cancelConnection()
	// Keep an application-level heartbeat comfortably below common reverse proxy
	// idle timeouts. Relay also sends protocol-level ping frames, but this
	// heartbeat updates the agent registry's last-seen timestamp.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		//如果进行退出去了，优雅的关闭
		case <-ctx.Done():
			_ = a.writeMessage(conn, protocol.TypeAgentOffline, map[string]string{"status": "offline"})
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "agent stopped"))
			return true, nil
			//如果存在了error
		case err := <-readDone:
			return true, err
			//定时到了，发送消息
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
				Task: subagentTaskPayload(task), RunID: task.CurrentRunID,
			}); err != nil {
				log.Printf("send subagent task event failed: %v", err)
				return
			}
		}
	}
}

func (a *Agent) forwardSubagentTaskEvents(ctx context.Context, conn *websocket.Conn, source SubagentTaskEventSource) {
	for {
		events, unsubscribe := source.SubscribeTaskEvents("", a.lastTaskEventSequence.Load(), 32)
		closed := a.forwardSubagentTaskEventBatch(ctx, conn, events)
		unsubscribe()
		if !closed || ctx.Err() != nil {
			return
		}
		if err := a.forwardSubagentTaskSnapshot(ctx, conn, source); err != nil {
			log.Printf("send subagent task snapshot failed: %v", err)
			return
		}
	}
}

func (a *Agent) forwardSubagentTaskEventBatch(ctx context.Context, conn *websocket.Conn, events <-chan subagentport.TaskEvent) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case event, ok := <-events:
			if !ok {
				return true
			}
			if err := a.writeRemoteMessage(conn, protocol.TypeSubagentTaskEvent, newRequestID(), event.Task.ParentSessionID, subagentEventPayload(event)); err != nil {
				log.Printf("send subagent task event failed: %v", err)
				return false
			}
			a.advanceTaskEventSequence(event.Sequence)
		}
	}
}

func (a *Agent) forwardSubagentTaskSnapshot(ctx context.Context, conn *websocket.Conn, source SubagentTaskEventSource) error {
	if a.subagentService == nil {
		return errors.New("subagent service is unavailable")
	}
	tail, ok := source.(SubagentTaskEventTailSource)
	if !ok {
		return errors.New("subagent event source cannot provide a snapshot cursor")
	}
	// Capture the cursor before loading tasks. Events published after this
	// point remain eligible for the next subscription instead of being skipped.
	latest := tail.LatestTaskEventSequence()
	result, err := a.subagentService.List(ctx, subagentcommand.ListTasks{Limit: 500})
	if err != nil {
		return err
	}
	for _, task := range result.Items {
		if err := a.writeRemoteMessage(conn, protocol.TypeSubagentTaskEvent, newRequestID(), task.ParentSessionID, protocol.SubagentTaskResultPayload{
			Task: subagentTaskPayload(task), RunID: task.CurrentRunID, Kind: "task.snapshot", Message: "subagent task snapshot",
		}); err != nil {
			return err
		}
	}
	a.advanceTaskEventSequence(latest)
	return nil
}

func (a *Agent) advanceTaskEventSequence(sequence uint64) {
	if sequence == 0 {
		return
	}
	for {
		current := a.lastTaskEventSequence.Load()
		if sequence <= current || a.lastTaskEventSequence.CompareAndSwap(current, sequence) {
			return
		}
	}
}

func (a *Agent) readLoop(ctx context.Context, conn *websocket.Conn, done chan<- error) {
	a.readLoopWithConnectionContext(ctx, ctx, conn, done)
}

func (a *Agent) readLoopWithConnectionContext(ctx, connectionCtx context.Context, conn *websocket.Conn, done chan<- error) {
	waitCtx, cancel := context.WithCancel(connectionCtx)
	defer cancel()
	for {
		var message protocol.Message
		_, raw, readErr := conn.ReadMessage()
		if readErr == nil {
			readErr = json.Unmarshal(raw, &message)
		}
		if err := readErr; err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				done <- nil
				return
			}
			done <- fmt.Errorf("read relay message failed: %w", err)
			return
		}

		if protocol.IsAssetUploadRequest(message.Type) && len(raw) > protocol.AssetUploadMaxJSONBytes {
			if writeErr := a.writeHandlerError(conn, message, errors.New("upload JSON exceeds 524288 bytes")); writeErr != nil {
				done <- writeErr
				return
			}
			continue
		}
		if message.Type == protocol.TypeAssetImagePreview && len(raw) > protocol.AssetImagePreviewRequestMaxJSONBytes {
			if writeErr := a.writeHandlerError(conn, message, errors.New("image preview request is too large")); writeErr != nil {
				done <- writeErr
				return
			}
			continue
		}
		if message.Type == protocol.TypeMCPDetail && len(raw) > protocol.MCPDetailRequestMaxJSONBytes {
			if writeErr := a.writeHandlerError(conn, message, errors.New("mcp detail request is too large")); writeErr != nil {
				done <- writeErr
				return
			}
			continue
		}
		fmt.Printf("relay message: type=%s request=%s\n", message.Type, message.RequestID)
		if waitCtx.Err() != nil {
			done <- waitCtx.Err()
			return
		}
		if err := a.dispatchRelayMessage(ctx, waitCtx, conn, message); err != nil {
			if writeErr := a.writeHandlerError(conn, message, err); writeErr != nil {
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
	//用户发起一次对话
	case protocol.TypeUserMessage:
		return a.startAsyncOperation(ctx, conn, message, a.processUserMessage)
	case protocol.TypeSessionRegenerate:
		return a.startAsyncOperation(ctx, conn, message, a.processRegenerateMessage)
	case protocol.TypePermissionResult:
		if err := a.handlePermissionResultContext(ctx, message); err != nil {
			return err
		}
		return a.writeRemoteMessage(conn, protocol.TypePermissionResult, message.RequestID, message.SessionID, json.RawMessage(message.Payload))
	case protocol.TypeUserQuestionAnswer:
		return a.handleUserQuestionAnswerContext(ctx, message)
	case protocol.TypeUserQuestionList:
		return a.handleUserQuestionList(conn, message)
	case protocol.TypeSessionList:
		return a.handleSessionList(ctx, conn, message)
	case protocol.TypeSessionNew:
		return a.handleSessionNew(ctx, conn, message)
	case protocol.TypeSessionLoad:
		return a.handleSessionLoad(ctx, conn, message)
	case protocol.TypeSessionRename:
		return a.handleSessionRename(ctx, conn, message)
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
		return a.startAsyncOperation(ctx, conn, message, a.processPlanExecuteMessage)
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
	case protocol.TypeIntentConfigQuery:
		return a.handleIntentConfigQuery(ctx, conn, message)
	case protocol.TypeIntentConfigSet:
		return a.handleIntentConfigSet(ctx, conn, message)
	case protocol.TypeIntentConfigTest:
		return a.handleIntentConfigTest(ctx, conn, message)
	case protocol.TypeIntentTraceList:
		return a.handleIntentTraceList(ctx, conn, message)
	case protocol.TypeIntentTraceGet:
		return a.handleIntentTraceGet(ctx, conn, message)
	case protocol.TypeIntentTraceClear:
		return a.handleIntentTraceClear(ctx, conn, message)
	case protocol.TypeSkillList:
		return a.handleSkillList(ctx, conn, message)
	case protocol.TypeSkillReload:
		return a.handleSkillReload(ctx, conn, message)
	case protocol.TypePluginList:
		return a.handlePluginList(ctx, conn, message)
	case protocol.TypePluginReload:
		a.enqueueCatalogMutation(ctx, conn, message, a.handlePluginReload)
	case protocol.TypeMCPList:
		return a.handleMCPList(conn, message)
	case protocol.TypeMCPDetail:
		return a.handleMCPDetail(conn, message)
	case protocol.TypeMCPReload:
		a.enqueueCatalogMutation(ctx, conn, message, a.handleMCPReload)
	case protocol.TypePluginEnable:
		a.enqueueCatalogMutation(ctx, conn, message, func(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
			return a.handlePluginToggle(ctx, conn, message, true)
		})
	case protocol.TypePluginDisable:
		a.enqueueCatalogMutation(ctx, conn, message, func(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
			return a.handlePluginToggle(ctx, conn, message, false)
		})
	case protocol.TypeAssetUploadBegin, protocol.TypeAssetUploadChunk, protocol.TypeAssetUploadStatus, protocol.TypeAssetUploadComplete, protocol.TypeAssetUploadCancel:
		return a.handleAssetUpload(ctx, conn, message)
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
		return a.startAsyncOperation(ctx, conn, message, a.processAIMemoryDreamRun)
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
	case protocol.TypeSubagentTaskDetail:
		return a.handleSubagentTaskDetail(ctx, conn, message)
	case protocol.TypeSubagentTaskEvents:
		return a.handleSubagentTaskEvents(ctx, conn, message)
	case protocol.TypeSubagentTaskMessage:
		return a.handleSubagentTaskMessage(ctx, conn, message)
	case protocol.TypeSubagentTaskFollowup:
		return a.handleSubagentTaskFollowup(ctx, conn, message)
	case protocol.TypeSubagentTaskWait:
		if a.subagentService == nil {
			return errors.New("subagent service is unavailable")
		}
		if a.activeSubagentWaits.Add(1) > 32 {
			a.activeSubagentWaits.Add(-1)
			return errors.New("too many pending subagent waits")
		}
		a.tasks.Add(1)
		go func() {
			defer a.tasks.Done()
			defer a.activeSubagentWaits.Add(-1)
			if err := a.handleSubagentTaskWait(ctx, conn, message); err != nil {
				_ = a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.ErrorPayload{Message: err.Error()})
			}
		}()
	case protocol.TypeSubagentTaskCancel:
		return a.handleSubagentTaskCancel(ctx, conn, message)
	case protocol.TypeSubagentTaskApply:
		return a.handleSubagentTaskApply(ctx, conn, message)
	case protocol.TypeSubagentTaskDiscard:
		return a.handleSubagentTaskDiscard(ctx, conn, message)
	case protocol.TypeSubagentTaskResume:
		return a.startAsyncOperation(ctx, conn, message, a.processSubagentTaskResume)
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
	// start 在状态锁内原子判断并登记运行任务；运行中的追加消息可以立即入队。
	runtime := a.runtimes.get(sessionID)

	runCtx, cancel, ok := runtime.start(ctx)
	// 此时会话已经有对话在生成了。
	if !ok {
		a.enqueueRunningTurnInput(ctx, conn, message, sessionID)
		return
	}
	// 当前任务结束后清除运行状态并取消其 Context。
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

func (a *Agent) writeJSON(conn *websocket.Conn, message protocol.Message) error {
	// reasoning、answer、tool event 可能由不同回调触发，WebSocket 写操作必须串行化。
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	if message.Type == protocol.TypeAssetImagePreviewResult || message.Type == protocol.TypeMCPDetailResult {
		// A stalled thumbnail or catalog write must release the shared socket writer.
		if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		defer conn.SetWriteDeadline(time.Time{})
	}
	return conn.WriteJSON(message)
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
