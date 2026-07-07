package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

type Server struct {
	addr       string
	upgrader   websocket.Upgrader
	agentLock  sync.RWMutex
	agents     map[string]*agentEntry
	bindings   map[string]string
	authStore  AuthStore
	clientLock sync.RWMutex
	clients    map[string]*clientEntry
}

func NewServer(addr string) *Server {
	if addr == "" {
		addr = ":8080"
	}

	return &Server{
		addr:      addr,
		agents:    make(map[string]*agentEntry),
		bindings:  make(map[string]string),
		authStore: NewMemoryAuthStore(),
		clients:   make(map[string]*clientEntry),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *Server) SetAuthStore(store AuthStore) {
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
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/agents", s.handleAgents)
	mux.HandleFunc("/pair", s.handlePair)
	mux.HandleFunc("/authorizations", s.handleAuthorizations)
	mux.HandleFunc("/authorizations/revoke", s.handleRevokeAuthorization)
	mux.HandleFunc("/ws/agent", s.handleAgent)
	mux.HandleFunc("/ws/client", s.handleClient)
	mux.Handle("/", s.webHandler())
	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		} else {
			w.Header().Add("Vary", "Origin")
		}

		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(agentsResponse{Agents: s.listAgents()}); err != nil {
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
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("%s websocket upgrade failed: %v", role, err)
		return
	}
	defer conn.Close()
	peer := newPeer(conn)

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
		var message protocol.Message
		if err := conn.ReadJSON(&message); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			log.Printf("%s read failed: %v", role, err)
			return
		}

		log.Printf("%s message: type=%s request=%s user=%s device=%s session=%s", role, message.Type, message.RequestID, message.UserID, message.DeviceID, message.SessionID)
		if err := s.handleRemoteMessage(peer, role, remoteAddr, message, &agentUserID, &agentDeviceID); err != nil {
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

func (s *Server) handleRemoteMessage(p *peer, role string, remoteAddr string, message protocol.Message, agentUserID *string, agentDeviceID *string) error {
	switch role {
	case "agent":
		return s.handleAgentMessage(p, remoteAddr, message, agentUserID, agentDeviceID)
	case "client":
		return s.handleClientMessage(p, remoteAddr, message)
	default:
		return nil
	}
}

func (s *Server) handleAgentMessage(p *peer, remoteAddr string, message protocol.Message, agentUserID *string, agentDeviceID *string) error {
	switch message.Type {
	case protocol.TypeAgentOnline:
		payload, err := protocol.DecodePayload[protocol.AgentOnlinePayload](message)
		if err != nil {
			return fmt.Errorf("decode agent online failed: %w", err)
		}
		*agentUserID = message.UserID
		*agentDeviceID = message.DeviceID
		s.registerAgent(p, message.UserID, message.DeviceID, payload.BindCode, remoteAddr)
		log.Printf("agent registered: user=%s device=%s", message.UserID, message.DeviceID)
	case protocol.TypeHeartbeat:
		s.touchAgent(message.UserID, message.DeviceID)
	case protocol.TypeAgentOffline:
		s.unregisterAgent(p, message.UserID, message.DeviceID)
		*agentUserID = ""
		*agentDeviceID = ""
		log.Printf("agent unregistered: user=%s device=%s", message.UserID, message.DeviceID)
	case protocol.TypeAssistantDelta, protocol.TypeAssistantDone, protocol.TypeToolCall, protocol.TypeToolResult, protocol.TypePermissionAsk, protocol.TypeSessionListResult, protocol.TypeSessionChanged, protocol.TypeSessionDeleteResult, protocol.TypeSessionRestoreResult, protocol.TypeSessionHistoryResult, protocol.TypeSessionHistoryMetaResult, protocol.TypeSessionHistoryDeltaResult, protocol.TypeSessionPermissionSetResult, protocol.TypeSessionContextSetResult, protocol.TypeSessionCompactResult, protocol.TypeSessionPauseResult, protocol.TypeModelListResult, protocol.TypeModelSwitchResult, protocol.TypeSkillListResult, protocol.TypeSkillReloadResult, protocol.TypeAssetListResult, protocol.TypeFileListResult, protocol.TypeFileReadResult, protocol.TypeChangesListResult, protocol.TypeChangeDiffResult, protocol.TypeChangeRevertResult, protocol.TypeHistoryListResult, protocol.TypeHistoryDiffResult, protocol.TypeHistoryRevertResult, protocol.TypeError:
		return s.forwardToClient(message)
	}

	return nil
}

func (s *Server) handleClientMessage(p *peer, remoteAddr string, message protocol.Message) error {
	switch message.Type {
	case protocol.TypeUserMessage, protocol.TypePermissionResult, protocol.TypeSessionList, protocol.TypeSessionNew, protocol.TypeSessionLoad, protocol.TypeSessionDelete, protocol.TypeSessionRestore, protocol.TypeSessionHistory, protocol.TypeSessionHistoryMeta, protocol.TypeSessionHistoryDelta, protocol.TypeSessionPermissionSet, protocol.TypeSessionContextSet, protocol.TypeSessionCompact, protocol.TypeSessionPause, protocol.TypeSessionRegenerate, protocol.TypeModelList, protocol.TypeModelSwitch, protocol.TypeSkillList, protocol.TypeSkillReload, protocol.TypeAssetList, protocol.TypeFileList, protocol.TypeFileRead, protocol.TypeChangesList, protocol.TypeChangeDiff, protocol.TypeChangeRevert, protocol.TypeHistoryList, protocol.TypeHistoryDiff, protocol.TypeHistoryRevert:
		if !s.validateClientToken(message.UserID, message.DeviceID, message.ClientToken) {
			return fmt.Errorf("client token is invalid or expired")
		}
		s.registerClient(message.RequestID, message.Type, p, message.UserID, message.DeviceID, remoteAddr)
		return s.forwardToAgent(message)
	case protocol.TypeHeartbeat:
		s.touchClient(message.RequestID)
	}

	return nil
}

func (s *Server) forwardToAgent(message protocol.Message) error {
	agent := s.getAgent(message.UserID, message.DeviceID)
	if agent == nil {
		return fmt.Errorf("agent is not online: user=%s device=%s", message.UserID, message.DeviceID)
	}

	return agent.peer.writeJSON(message)
}

func (s *Server) forwardToClient(message protocol.Message) error {
	client := s.getClient(message.RequestID)
	if client == nil {
		return fmt.Errorf("client request is not online: request=%s", message.RequestID)
	}

	if isTerminalResponseForRequest(client.RequestType, message.Type) {
		defer s.unregisterClient(message.RequestID)
	}
	return client.peer.writeJSON(message)
}

func isTerminalResponseForRequest(requestType protocol.MessageType, responseType protocol.MessageType) bool {
	if responseType == protocol.TypeError {
		return true
	}

	switch requestType {
	case protocol.TypeUserMessage, protocol.TypeSessionRegenerate:
		return responseType == protocol.TypeAssistantDone
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
	case protocol.TypeSessionContextSet:
		return responseType == protocol.TypeSessionContextSetResult
	case protocol.TypeSessionCompact:
		return responseType == protocol.TypeSessionCompactResult
	case protocol.TypeSessionPause:
		return responseType == protocol.TypeSessionPauseResult
	case protocol.TypeModelList:
		return responseType == protocol.TypeModelListResult
	case protocol.TypeModelSwitch:
		return responseType == protocol.TypeModelSwitchResult
	case protocol.TypeSkillList:
		return responseType == protocol.TypeSkillListResult
	case protocol.TypeSkillReload:
		return responseType == protocol.TypeSkillReloadResult
	case protocol.TypeAssetList:
		return responseType == protocol.TypeAssetListResult
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

func URL(addr string) string {
	if addr == "" {
		addr = ":8080"
	}
	return fmt.Sprintf("http://localhost%s", addr)
}
