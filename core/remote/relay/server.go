package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	authorizationport "myai/core/port/authorization"
	"myai/core/remote/protocol"
)

const (
	websocketPingInterval = 20 * time.Second
	websocketPongWait     = 60 * time.Second
	websocketWriteWait    = 10 * time.Second
)

type Server struct {
	// Relay 只管理连接、配对和消息路由，不依赖 ChatService，也不执行模型或工具逻辑。
	addr             string
	upgrader         websocket.Upgrader
	agentLock        sync.RWMutex
	agents           map[string]*agentEntry
	bindings         map[string]string
	authStore        authorizationport.Store
	clientLock       sync.RWMutex
	clients          map[string]*clientEntry
	connections      map[*peer]*clientConnection
	agentCredentials map[string]string
	origins          map[string]struct{}
	loopbackOrigins  map[string]struct{}
	allowAllOrigins  bool
}

type Option func(*Server)

type AgentCredential struct {
	UserID   string
	DeviceID string
	Token    string
}

func WithAgentCredentials(credentials ...AgentCredential) Option {
	return func(server *Server) {
		for _, credential := range credentials {
			userID := strings.TrimSpace(credential.UserID)
			deviceID := strings.TrimSpace(credential.DeviceID)
			token := strings.TrimSpace(credential.Token)
			if userID == "" || deviceID == "" || token == "" {
				continue
			}
			server.agentCredentials[agentKey(userID, deviceID)] = token
		}
	}
}

func WithAllowedOrigins(origins ...string) Option {
	return func(server *Server) {
		for _, origin := range origins {
			if strings.TrimSpace(origin) == "*" {
				server.allowAllOrigins = true
				continue
			}
			if pattern, ok := normalizeLoopbackOriginPattern(origin); ok {
				server.loopbackOrigins[pattern] = struct{}{}
				continue
			}
			if normalized, ok := normalizeOrigin(origin); ok {
				server.origins[normalized] = struct{}{}
			}
		}
	}
}

func NewServer(addr string, authStore authorizationport.Store, options ...Option) *Server {
	if addr == "" {
		addr = ":8080"
	}

	server := &Server{
		addr:             addr,
		agents:           make(map[string]*agentEntry),
		bindings:         make(map[string]string),
		authStore:        authStore,
		clients:          make(map[string]*clientEntry),
		connections:      make(map[*peer]*clientConnection),
		agentCredentials: make(map[string]string),
		origins:          make(map[string]struct{}),
		loopbackOrigins:  make(map[string]struct{}),
	}
	for _, option := range options {
		if option != nil {
			option(server)
		}
	}
	server.upgrader = websocket.Upgrader{CheckOrigin: server.originAllowed}
	return server
}

func (s *Server) SetAuthStore(store authorizationport.Store) {
	if store != nil {
		s.authStore = store
	}
}

func (s *Server) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:              s.addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("relay server listening on %s", s.addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) routes() http.Handler {
	// HTTP 用于健康检查、配对和授权；聊天等实时消息统一走 Agent/Client WebSocket。
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/agents", s.handleAgents)
	mux.HandleFunc("/pair", s.handlePair)
	mux.HandleFunc("/authorizations", s.handleAuthorizations)
	mux.HandleFunc("/authorizations/revoke", s.handleRevokeAuthorization)
	mux.HandleFunc("/ws/agent", s.handleAgent)
	mux.HandleFunc("/ws/client", s.handleClient)
	mux.Handle("/", s.webHandler())
	return s.corsMiddleware(mux)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			if !s.originAllowed(r) {
				http.Error(w, "origin is not allowed", http.StatusForbidden)
				return
			}
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-MyAI-Agent-User-ID, X-MyAI-Agent-Device-ID")
		w.Header().Set("Access-Control-Max-Age", "600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	agents := s.listAgents()
	if _, ok := s.authenticateAgentRequest(r); !ok {
		userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
		deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
		if !s.validateClientToken(userID, deviceID, clientTokenFromRequest(r)) {
			http.Error(w, "agent or paired client authentication required", http.StatusUnauthorized)
			return
		}
		agents = nil
		if agent, found := s.agentInfo(userID, deviceID); found {
			agents = []AgentInfo{agent}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(agentsResponse{Agents: agents}); err != nil {
		log.Printf("write agents response failed: %v", err)
	}
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	s.handleWebSocket(w, r, "agent")
}

func (s *Server) handleClient(w http.ResponseWriter, r *http.Request) {
	s.handleWebSocket(w, r, "client")
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request, role string) {
	authenticatedAgent := agentIdentity{}
	if role == "agent" {
		var ok bool
		authenticatedAgent, ok = s.authenticateAgentRequest(r)
		if !ok {
			http.Error(w, "agent authentication required", http.StatusUnauthorized)
			return
		}
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("%s websocket upgrade failed: %v", role, err)
		return
	}
	defer conn.Close()
	peer := newPeer(conn)
	_ = conn.SetReadDeadline(time.Now().Add(websocketPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(websocketPongWait))
	})
	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		ticker := time.NewTicker(websocketPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := peer.writeControl(websocket.PingMessage, nil, time.Now().Add(websocketWriteWait)); err != nil {
					_ = peer.close()
					return
				}
			case <-stopPing:
				return
			}
		}
	}()

	remoteAddr := r.RemoteAddr
	log.Printf("%s connected: %s", role, remoteAddr)
	defer log.Printf("%s disconnected: %s", role, remoteAddr)

	var agentUserID string
	var agentDeviceID string
	defer func() {
		if role == "agent" {
			s.unregisterAgent(peer, agentUserID, agentDeviceID)
		}
		if role == "client" {
			s.unregisterClientPeer(peer)
		}
	}()

	for {
		// Relay 保持业务消息原样，只在注册连接和路由失败时生成自己的 ack/error。
		var message protocol.Message
		if err := conn.ReadJSON(&message); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			log.Printf("%s read failed: %v", role, err)
			return
		}
		// A compliant client normally answers Relay pings with a Pong, but some
		// mobile/proxy combinations only reliably deliver application messages.
		// Count any valid message as activity as well, so an active heartbeat or
		// request cannot be closed by the read deadline while its Pong is delayed.
		_ = conn.SetReadDeadline(time.Now().Add(websocketPongWait))

		log.Printf("%s message: type=%s request=%s user=%s device=%s session=%s", role, message.Type, message.RequestID, message.UserID, message.DeviceID, message.SessionID)
		if err := s.handleRemoteMessage(peer, role, remoteAddr, message, authenticatedAgent, &agentUserID, &agentDeviceID); err != nil {
			log.Printf("%s handle message failed: %v", role, err)
			if writeErr := writeError(peer, message, err.Error()); writeErr != nil {
				log.Printf("%s error response failed: %v", role, writeErr)
				return
			}
			continue
		}

		if err := writeAck(peer, role, message); err != nil {
			log.Printf("%s ack failed: %v", role, err)
			return
		}
	}
}

func (s *Server) handleRemoteMessage(p *peer, role string, remoteAddr string, message protocol.Message, authenticatedAgent agentIdentity, agentUserID *string, agentDeviceID *string) error {
	switch role {
	case "agent":
		return s.handleAgentMessage(p, remoteAddr, message, authenticatedAgent, agentUserID, agentDeviceID)
	case "client":
		return s.handleClientMessage(p, remoteAddr, message)
	default:
		return nil
	}
}

func (s *Server) handleAgentMessage(p *peer, remoteAddr string, message protocol.Message, authenticatedAgent agentIdentity, agentUserID *string, agentDeviceID *string) error {
	if message.Type != protocol.TypeAgentOnline {
		if *agentUserID == "" || *agentDeviceID == "" {
			return errors.New("agent must register before sending messages")
		}
		if message.UserID != *agentUserID || message.DeviceID != *agentDeviceID {
			return errors.New("agent message identity does not match the authenticated connection")
		}
		if !s.isCurrentAgentPeer(p, *agentUserID, *agentDeviceID) {
			return errors.New("agent connection has been superseded")
		}
	}
	switch message.Type {
	case protocol.TypeAgentOnline:
		// AgentOnline 建立 user/device 到连接的路由，之后手机请求才能定位到这台电脑。
		payload, err := protocol.DecodePayload[protocol.AgentOnlinePayload](message)
		if err != nil {
			return fmt.Errorf("decode agent online failed: %w", err)
		}
		if strings.TrimSpace(message.UserID) == "" || strings.TrimSpace(message.DeviceID) == "" {
			return errors.New("agent user and device ids are required")
		}
		if message.UserID != authenticatedAgent.UserID || message.DeviceID != authenticatedAgent.DeviceID {
			return errors.New("agent registration identity does not match its credential")
		}
		if *agentUserID != "" && (*agentUserID != message.UserID || *agentDeviceID != message.DeviceID) {
			return errors.New("agent connection cannot change identity")
		}
		*agentUserID = message.UserID
		*agentDeviceID = message.DeviceID
		superseded := s.registerAgent(p, message.UserID, message.DeviceID, payload.BindCode, remoteAddr)
		if superseded != nil {
			if err := superseded.close(); err != nil {
				log.Printf("close superseded agent connection failed: %v", err)
			}
		}
		log.Printf("agent registered: user=%s device=%s", message.UserID, message.DeviceID)
	case protocol.TypeHeartbeat:
		s.touchAgent(message.UserID, message.DeviceID)
	case protocol.TypeAgentOffline:
		s.unregisterAgent(p, message.UserID, message.DeviceID)
		*agentUserID = ""
		*agentDeviceID = ""
		log.Printf("agent unregistered: user=%s device=%s", message.UserID, message.DeviceID)
	case protocol.TypeSubagentTaskEvent:
		return s.forwardEventToClients(message)
	case protocol.TypeAssistantDelta, protocol.TypeAssistantDone, protocol.TypeAgentRunStarted, protocol.TypeAgentRunEvent, protocol.TypeAgentRunCompleted, protocol.TypeAgentRunListResult, protocol.TypeToolCall, protocol.TypeToolResult, protocol.TypePermissionAsk, protocol.TypeSessionListResult, protocol.TypeSessionChanged, protocol.TypeSessionDeleteResult, protocol.TypeSessionRestoreResult, protocol.TypeSessionHistoryResult, protocol.TypeSessionHistoryMetaResult, protocol.TypeSessionHistoryDeltaResult, protocol.TypeSessionPermissionSetResult, protocol.TypeSessionModeSetResult, protocol.TypeSessionPlanExecuteUpdate, protocol.TypeSessionPlanExecuteResult, protocol.TypeSessionContextQueryResult, protocol.TypeSessionContextSetResult, protocol.TypeSessionRAGSetResult, protocol.TypeSessionGenerationQueryResult, protocol.TypeSessionGenerationSetResult, protocol.TypeSessionStyleSetResult, protocol.TypeSessionCompactResult, protocol.TypeSessionPauseResult, protocol.TypeModelListResult, protocol.TypeModelSwitchResult, protocol.TypeModelConfigAddResult, protocol.TypeModelConfigTestResult, protocol.TypeModelConfigMutationResult, protocol.TypeSkillListResult, protocol.TypeSkillReloadResult, protocol.TypePluginListResult, protocol.TypePluginReloadResult, protocol.TypePluginMutationResult, protocol.TypeAssetListResult, protocol.TypeKnowledgeCatalogListResult, protocol.TypeKnowledgeCatalogMutationResult, protocol.TypeKnowledgeDocumentListResult, protocol.TypeKnowledgeDocumentMutationResult, protocol.TypeKnowledgeProfileListResult, protocol.TypeKnowledgeSearchPreviewResult, protocol.TypeAIMemoryListResult, protocol.TypeAIMemoryMutationResult, protocol.TypeAIMemoryCandidateListResult, protocol.TypeAIMemoryCandidateMutationResult, protocol.TypeAIMemoryExtractionJobListResult, protocol.TypeAIMemoryExtractionJobRetryResult, protocol.TypeAIMemoryDreamRunResult, protocol.TypeAIMemoryDreamListResult, protocol.TypeSubagentDefinitionListResult, protocol.TypeSubagentDefinitionMutationResult, protocol.TypeSubagentTaskListResult, protocol.TypeSubagentTaskWaitResult, protocol.TypeSubagentTaskResult, protocol.TypeSubagentTaskResumeResult, protocol.TypeFileListResult, protocol.TypeFileReadResult, protocol.TypeChangesListResult, protocol.TypeChangeDiffResult, protocol.TypeChangeRevertResult, protocol.TypeHistoryListResult, protocol.TypeHistoryDiffResult, protocol.TypeHistoryRevertResult, protocol.TypeError:
		return s.forwardToClient(message)
	case protocol.TypeIntentConfigQueryResult, protocol.TypeIntentConfigSetResult, protocol.TypeIntentConfigTestResult, protocol.TypeIntentTraceListResult, protocol.TypeIntentTraceGetResult, protocol.TypeIntentTraceClearResult:
		return s.forwardToClient(message)
	default:
		return fmt.Errorf("unsupported agent message type: %s", message.Type)
	}

	return nil
}

func (s *Server) handleClientMessage(p *peer, remoteAddr string, message protocol.Message) error {
	switch message.Type {
	case protocol.TypePermissionResult:
		if !s.validateClientToken(message.UserID, message.DeviceID, message.ClientToken) {
			return fmt.Errorf("client token is invalid or expired")
		}
		// permission_result 属于原聊天请求，不能覆盖 request_id 已登记的请求类型，
		// 否则后续 assistant_done 无法按 user_message 终态释放路由。
		s.registerClientConnection(p, message.UserID, message.DeviceID, message.ClientToken, remoteAddr)
		if !s.rebindClientRequest(message.RequestID, message.UserID, message.DeviceID, message.ClientToken, p) {
			return fmt.Errorf("client request is not online: request=%s", message.RequestID)
		}
		return s.forwardToAgent(message)
	case protocol.TypeUserMessage, protocol.TypeAgentRunList, protocol.TypeSessionList, protocol.TypeSessionNew, protocol.TypeSessionLoad, protocol.TypeSessionDelete, protocol.TypeSessionRestore, protocol.TypeSessionHistory, protocol.TypeSessionHistoryMeta, protocol.TypeSessionHistoryDelta, protocol.TypeSessionPermissionSet, protocol.TypeSessionModeSet, protocol.TypeSessionPlanExecute, protocol.TypeSessionContextQuery, protocol.TypeSessionContextSet, protocol.TypeSessionRAGSet, protocol.TypeSessionGenerationQuery, protocol.TypeSessionGenerationSet, protocol.TypeSessionStyleSet, protocol.TypeSessionCompact, protocol.TypeSessionPause, protocol.TypeSessionRegenerate, protocol.TypeModelList, protocol.TypeModelSwitch, protocol.TypeModelConfigAdd, protocol.TypeModelConfigTest, protocol.TypeModelConfigUpdate, protocol.TypeModelConfigDelete, protocol.TypeModelConfigEnabledSet, protocol.TypeModelConfigDefaultSet, protocol.TypeSkillList, protocol.TypeSkillReload, protocol.TypePluginList, protocol.TypePluginReload, protocol.TypePluginEnable, protocol.TypePluginDisable, protocol.TypeAssetList, protocol.TypeFileList, protocol.TypeKnowledgeCatalogList, protocol.TypeKnowledgeCategoryCreate, protocol.TypeKnowledgeCategoryMove, protocol.TypeKnowledgeCategoryDelete, protocol.TypeKnowledgeBaseCreate, protocol.TypeKnowledgeBaseUpdate, protocol.TypeKnowledgeBaseDelete, protocol.TypeKnowledgeDocumentList, protocol.TypeKnowledgeDocumentIngest, protocol.TypeKnowledgeDocumentRetry, protocol.TypeKnowledgeDocumentDelete, protocol.TypeKnowledgeProfileList, protocol.TypeKnowledgeSearchPreview, protocol.TypeAIMemoryList, protocol.TypeAIMemoryCreate, protocol.TypeAIMemoryUpdate, protocol.TypeAIMemoryDelete, protocol.TypeAIMemoryRestore, protocol.TypeAIMemoryCandidateList, protocol.TypeAIMemoryCandidateApprove, protocol.TypeAIMemoryCandidateReject, protocol.TypeAIMemoryExtractionJobList, protocol.TypeAIMemoryExtractionJobRetry, protocol.TypeAIMemoryDreamRun, protocol.TypeAIMemoryDreamList, protocol.TypeSubagentDefinitionList, protocol.TypeSubagentDefinitionCreate, protocol.TypeSubagentDefinitionUpdate, protocol.TypeSubagentDefinitionDelete, protocol.TypeSubagentTaskList, protocol.TypeSubagentTaskCheck, protocol.TypeSubagentTaskMessage, protocol.TypeSubagentTaskFollowup, protocol.TypeSubagentTaskWait, protocol.TypeSubagentTaskCancel, protocol.TypeSubagentTaskApply, protocol.TypeSubagentTaskDiscard, protocol.TypeSubagentTaskResume, protocol.TypeFileRead, protocol.TypeChangesList, protocol.TypeChangeDiff, protocol.TypeChangeRevert, protocol.TypeHistoryList, protocol.TypeHistoryDiff, protocol.TypeHistoryRevert:
		// 手机每次请求都校验 token，不能只信任客户端声明的 user/device。
		if !s.validateClientToken(message.UserID, message.DeviceID, message.ClientToken) {
			return fmt.Errorf("client token is invalid or expired")
		}
		if !s.registerClient(message.RequestID, message.Type, p, message.UserID, message.DeviceID, message.ClientToken, remoteAddr) {
			return fmt.Errorf("client request id is already owned by another client: request=%s", message.RequestID)
		}
		s.registerClientConnection(p, message.UserID, message.DeviceID, message.ClientToken, remoteAddr)
		return s.forwardToAgent(message)
	case protocol.TypeIntentConfigQuery, protocol.TypeIntentConfigSet, protocol.TypeIntentConfigTest, protocol.TypeIntentTraceList, protocol.TypeIntentTraceGet, protocol.TypeIntentTraceClear:
		if !s.validateClientToken(message.UserID, message.DeviceID, message.ClientToken) {
			return fmt.Errorf("client token is invalid or expired")
		}
		if !s.registerClient(message.RequestID, message.Type, p, message.UserID, message.DeviceID, message.ClientToken, remoteAddr) {
			return fmt.Errorf("client request id is already owned by another client: request=%s", message.RequestID)
		}
		s.registerClientConnection(p, message.UserID, message.DeviceID, message.ClientToken, remoteAddr)
		return s.forwardToAgent(message)
	case protocol.TypeHeartbeat:
		// Heartbeats also keep an idle mobile connection registered for push-style
		// events (for example subagent_task_event).  Validate the paired identity
		// before refreshing the connection; request_id is optional for heartbeats.
		// Keep accepting the legacy empty heartbeat emitted by older APKs while they
		// are being upgraded; it can only touch an already-registered request route.
		if message.UserID == "" && message.DeviceID == "" && message.ClientToken == "" {
			s.touchClient(message.RequestID)
			return nil
		}
		if !s.validateClientToken(message.UserID, message.DeviceID, message.ClientToken) {
			return fmt.Errorf("client token is invalid or expired")
		}
		s.registerClientConnection(p, message.UserID, message.DeviceID, message.ClientToken, remoteAddr)
		s.touchClientPeer(p)
	default:
		return fmt.Errorf("unsupported client message type: %s", message.Type)
	}

	return nil
}

func (s *Server) forwardToAgent(message protocol.Message) error {
	// request_id 和 session_id 原样保留，流式响应才能准确回到原请求和会话。
	agent := s.getAgent(message.UserID, message.DeviceID)
	if agent == nil {
		return fmt.Errorf("agent is not online: user=%s device=%s", message.UserID, message.DeviceID)
	}

	return agent.peer.writeJSON(message)
}

func (s *Server) forwardToClient(message protocol.Message) error {
	client, target, ok := s.clientPeerForMessage(
		message.RequestID,
		message.UserID,
		message.DeviceID,
	)
	if !ok {
		return fmt.Errorf("client request is not online: request=%s", message.RequestID)
	}

	if err := target.writeJSON(message); err != nil {
		// The TCP close can race with an agent response. Retire the stale peer
		// and retry once against a replacement connection for the same identity.
		s.unregisterClientPeer(target)
		_, replacement, found := s.clientPeerForMessage(
			message.RequestID,
			message.UserID,
			message.DeviceID,
		)
		if !found || replacement == target {
			return err
		}
		if err := replacement.writeJSON(message); err != nil {
			return err
		}
	}
	if isTerminalResponseForRequest(client.RequestType, message.Type) {
		s.unregisterClient(message.RequestID)
	}
	return nil
}

func (s *Server) forwardEventToClients(message protocol.Message) error {
	peers := s.getClientConnections(message.UserID, message.DeviceID)
	if len(peers) == 0 {
		return nil
	}
	var errs []error
	for _, peer := range peers {
		if err := peer.writeJSON(message); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func isTerminalResponseForRequest(requestType protocol.MessageType, responseType protocol.MessageType) bool {
	if responseType == protocol.TypeError {
		return true
	}

	switch requestType {
	case protocol.TypeUserMessage:
		return responseType == protocol.TypeAssistantDone || responseType == protocol.TypeUserMessageQueued
	case protocol.TypeSessionRegenerate:
		return responseType == protocol.TypeAssistantDone
	case protocol.TypeAgentRunList:
		return responseType == protocol.TypeAgentRunListResult
	case protocol.TypeSessionList:
		return responseType == protocol.TypeSessionListResult
	case protocol.TypeSessionNew, protocol.TypeSessionLoad:
		return responseType == protocol.TypeSessionChanged
	case protocol.TypeSessionDelete:
		return responseType == protocol.TypeSessionDeleteResult
	case protocol.TypeSessionRestore:
		return responseType == protocol.TypeSessionRestoreResult
	case protocol.TypeSessionHistory:
		return responseType == protocol.TypeSessionHistoryResult
	case protocol.TypeSessionHistoryMeta:
		return responseType == protocol.TypeSessionHistoryMetaResult
	case protocol.TypeSessionHistoryDelta:
		return responseType == protocol.TypeSessionHistoryDeltaResult
	case protocol.TypeSessionPermissionSet:
		return responseType == protocol.TypeSessionPermissionSetResult
	case protocol.TypeSessionModeSet:
		return responseType == protocol.TypeSessionModeSetResult
	case protocol.TypeSessionPlanExecute:
		return responseType == protocol.TypeSessionPlanExecuteResult
	case protocol.TypeSessionContextQuery:
		return responseType == protocol.TypeSessionContextQueryResult
	case protocol.TypeSessionContextSet:
		return responseType == protocol.TypeSessionContextSetResult
	case protocol.TypeSessionRAGSet:
		return responseType == protocol.TypeSessionRAGSetResult
	case protocol.TypeSessionGenerationQuery:
		return responseType == protocol.TypeSessionGenerationQueryResult
	case protocol.TypeSessionGenerationSet:
		return responseType == protocol.TypeSessionGenerationSetResult
	case protocol.TypeSessionStyleSet:
		return responseType == protocol.TypeSessionStyleSetResult
	case protocol.TypeSessionCompact:
		return responseType == protocol.TypeSessionCompactResult
	case protocol.TypeSessionPause:
		return responseType == protocol.TypeSessionPauseResult
	case protocol.TypeModelList:
		return responseType == protocol.TypeModelListResult
	case protocol.TypeModelSwitch:
		return responseType == protocol.TypeModelSwitchResult
	case protocol.TypeModelConfigAdd:
		return responseType == protocol.TypeModelConfigAddResult
	case protocol.TypeModelConfigTest:
		return responseType == protocol.TypeModelConfigTestResult
	case protocol.TypeModelConfigUpdate, protocol.TypeModelConfigDelete, protocol.TypeModelConfigEnabledSet, protocol.TypeModelConfigDefaultSet:
		return responseType == protocol.TypeModelConfigMutationResult
	case protocol.TypeIntentConfigQuery:
		return responseType == protocol.TypeIntentConfigQueryResult
	case protocol.TypeIntentConfigSet:
		return responseType == protocol.TypeIntentConfigSetResult
	case protocol.TypeIntentConfigTest:
		return responseType == protocol.TypeIntentConfigTestResult
	case protocol.TypeIntentTraceList:
		return responseType == protocol.TypeIntentTraceListResult
	case protocol.TypeIntentTraceGet:
		return responseType == protocol.TypeIntentTraceGetResult
	case protocol.TypeIntentTraceClear:
		return responseType == protocol.TypeIntentTraceClearResult
	case protocol.TypeSkillList:
		return responseType == protocol.TypeSkillListResult
	case protocol.TypeSkillReload:
		return responseType == protocol.TypeSkillReloadResult
	case protocol.TypePluginList:
		return responseType == protocol.TypePluginListResult
	case protocol.TypePluginReload:
		return responseType == protocol.TypePluginReloadResult
	case protocol.TypePluginEnable, protocol.TypePluginDisable:
		return responseType == protocol.TypePluginMutationResult
	case protocol.TypeAssetList:
		return responseType == protocol.TypeAssetListResult
	case protocol.TypeKnowledgeCatalogList:
		return responseType == protocol.TypeKnowledgeCatalogListResult
	case protocol.TypeKnowledgeCategoryCreate,
		protocol.TypeKnowledgeCategoryMove,
		protocol.TypeKnowledgeCategoryDelete,
		protocol.TypeKnowledgeBaseCreate,
		protocol.TypeKnowledgeBaseUpdate,
		protocol.TypeKnowledgeBaseDelete:
		return responseType == protocol.TypeKnowledgeCatalogMutationResult
	case protocol.TypeKnowledgeDocumentList:
		return responseType == protocol.TypeKnowledgeDocumentListResult
	case protocol.TypeKnowledgeDocumentIngest,
		protocol.TypeKnowledgeDocumentRetry,
		protocol.TypeKnowledgeDocumentDelete:
		return responseType == protocol.TypeKnowledgeDocumentMutationResult
	case protocol.TypeKnowledgeProfileList:
		return responseType == protocol.TypeKnowledgeProfileListResult
	case protocol.TypeKnowledgeSearchPreview:
		return responseType == protocol.TypeKnowledgeSearchPreviewResult
	case protocol.TypeAIMemoryList:
		return responseType == protocol.TypeAIMemoryListResult
	case protocol.TypeAIMemoryCreate, protocol.TypeAIMemoryUpdate, protocol.TypeAIMemoryDelete, protocol.TypeAIMemoryRestore:
		return responseType == protocol.TypeAIMemoryMutationResult
	case protocol.TypeAIMemoryCandidateList:
		return responseType == protocol.TypeAIMemoryCandidateListResult
	case protocol.TypeAIMemoryCandidateApprove, protocol.TypeAIMemoryCandidateReject:
		return responseType == protocol.TypeAIMemoryCandidateMutationResult
	case protocol.TypeAIMemoryExtractionJobList:
		return responseType == protocol.TypeAIMemoryExtractionJobListResult
	case protocol.TypeAIMemoryExtractionJobRetry:
		return responseType == protocol.TypeAIMemoryExtractionJobRetryResult
	case protocol.TypeAIMemoryDreamRun:
		return responseType == protocol.TypeAIMemoryDreamRunResult
	case protocol.TypeAIMemoryDreamList:
		return responseType == protocol.TypeAIMemoryDreamListResult
	case protocol.TypeSubagentDefinitionList:
		return responseType == protocol.TypeSubagentDefinitionListResult
	case protocol.TypeSubagentDefinitionCreate, protocol.TypeSubagentDefinitionUpdate, protocol.TypeSubagentDefinitionDelete:
		return responseType == protocol.TypeSubagentDefinitionMutationResult
	case protocol.TypeSubagentTaskList:
		return responseType == protocol.TypeSubagentTaskListResult
	case protocol.TypeSubagentTaskCheck, protocol.TypeSubagentTaskCancel, protocol.TypeSubagentTaskApply, protocol.TypeSubagentTaskDiscard:
		return responseType == protocol.TypeSubagentTaskResult
	case protocol.TypeSubagentTaskMessage:
		return responseType == protocol.TypeSubagentTaskResult
	case protocol.TypeSubagentTaskFollowup:
		return responseType == protocol.TypeSubagentTaskResult
	case protocol.TypeSubagentTaskWait:
		return responseType == protocol.TypeSubagentTaskWaitResult
	case protocol.TypeSubagentTaskResume:
		return responseType == protocol.TypeSubagentTaskResumeResult
	case protocol.TypeFileList:
		return responseType == protocol.TypeFileListResult
	case protocol.TypeFileRead:
		return responseType == protocol.TypeFileReadResult
	case protocol.TypeChangesList:
		return responseType == protocol.TypeChangesListResult
	case protocol.TypeChangeDiff:
		return responseType == protocol.TypeChangeDiffResult
	case protocol.TypeChangeRevert:
		return responseType == protocol.TypeChangeRevertResult
	case protocol.TypeHistoryList:
		return responseType == protocol.TypeHistoryListResult
	case protocol.TypeHistoryDiff:
		return responseType == protocol.TypeHistoryDiffResult
	case protocol.TypeHistoryRevert:
		return responseType == protocol.TypeHistoryRevertResult
	default:
		return false
	}
}

func writeAck(p *peer, role string, received protocol.Message) error {
	payload := map[string]string{
		"role":     role,
		"received": string(received.Type),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return p.writeJSON(protocol.Message{
		Type:      protocol.TypeHeartbeat,
		RequestID: received.RequestID,
		UserID:    received.UserID,
		DeviceID:  received.DeviceID,
		SessionID: received.SessionID,
		Payload:   data,
	})
}

func writeError(p *peer, received protocol.Message, text string) error {
	message, err := protocol.NewMessage(
		protocol.TypeError,
		received.RequestID,
		received.UserID,
		received.DeviceID,
		received.SessionID,
		protocol.ErrorPayload{Message: text},
	)
	if err != nil {
		return err
	}
	return p.writeJSON(message)
}
