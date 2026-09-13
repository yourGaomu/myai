package relay

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

// A client request may outlive the WebSocket that created it (for example when
// a mobile browser is backgrounded while the agent is waiting for permission).
// Keep the route for a bounded period so a newly authenticated connection can
// take it over, while still preventing abandoned requests from accumulating.
const clientRouteTTL = 30 * time.Minute

type peer struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func newPeer(conn *websocket.Conn) *peer {
	return &peer{conn: conn}
}

func (p *peer) writeJSON(value any) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.conn.WriteJSON(value)
}

func (p *peer) writeControl(messageType int, data []byte, deadline time.Time) error {
	if p == nil || p.conn == nil {
		return nil
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.conn.WriteControl(messageType, data, deadline)
}

func (p *peer) close() error {
	if p == nil || p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

type agentEntry struct {
	UserID      string
	DeviceID    string
	BindCode    string
	RemoteAddr  string
	ConnectedAt time.Time
	LastSeenAt  time.Time
	peer        *peer
}

type clientEntry struct {
	RequestID   string
	RequestType protocol.MessageType
	UserID      string
	DeviceID    string
	ClientID    string
	RemoteAddr  string
	ConnectedAt time.Time
	LastSeenAt  time.Time
	peer        *peer
}

type clientConnection struct {
	UserID     string
	DeviceID   string
	ClientID   string
	RemoteAddr string
	LastSeenAt time.Time
}

type AgentInfo struct {
	UserID      string    `json:"user_id"`
	DeviceID    string    `json:"device_id"`
	RemoteAddr  string    `json:"remote_addr"`
	ConnectedAt time.Time `json:"connected_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

type agentsResponse struct {
	Agents []AgentInfo `json:"agents"`
}

func agentKey(userID string, deviceID string) string {
	return fmt.Sprintf("%s/%s", userID, deviceID)
}

func (s *Server) registerAgent(p *peer, userID string, deviceID string, bindCode string, remoteAddr string) *peer {
	if userID == "" || deviceID == "" {
		return nil
	}

	now := time.Now()
	key := agentKey(userID, deviceID)

	s.agentLock.Lock()

	var superseded *peer
	if previous := s.agents[key]; previous != nil {
		if previous.BindCode != "" {
			delete(s.bindings, previous.BindCode)
		}
		if previous.peer != p {
			superseded = previous.peer
		}
	}

	s.agents[key] = &agentEntry{
		UserID:      userID,
		DeviceID:    deviceID,
		BindCode:    bindCode,
		RemoteAddr:  remoteAddr,
		ConnectedAt: now,
		LastSeenAt:  now,
		peer:        p,
	}

	if bindCode != "" {
		s.bindings[bindCode] = key
	}
	s.agentLock.Unlock()
	return superseded
}

func (s *Server) touchAgent(userID string, deviceID string) {
	if userID == "" || deviceID == "" {
		return
	}

	key := agentKey(userID, deviceID)

	s.agentLock.Lock()
	defer s.agentLock.Unlock()

	if agent := s.agents[key]; agent != nil {
		agent.LastSeenAt = time.Now()
	}
}

func (s *Server) unregisterAgent(p *peer, userID string, deviceID string) {
	if userID == "" || deviceID == "" {
		return
	}

	key := agentKey(userID, deviceID)

	s.agentLock.Lock()
	defer s.agentLock.Unlock()

	if agent := s.agents[key]; agent != nil && agent.peer == p {
		if agent.BindCode != "" {
			delete(s.bindings, agent.BindCode)
		}
		delete(s.agents, key)
	}
}

func (s *Server) getAgent(userID string, deviceID string) *agentEntry {
	if userID == "" || deviceID == "" {
		return nil
	}

	key := agentKey(userID, deviceID)

	s.agentLock.RLock()
	defer s.agentLock.RUnlock()

	return s.agents[key]
}

func (s *Server) isCurrentAgentPeer(p *peer, userID string, deviceID string) bool {
	if p == nil || userID == "" || deviceID == "" {
		return false
	}
	s.agentLock.RLock()
	defer s.agentLock.RUnlock()
	agent := s.agents[agentKey(userID, deviceID)]
	return agent != nil && agent.peer == p
}

func (s *Server) getAgentByBindCode(bindCode string) *agentEntry {
	if bindCode == "" {
		return nil
	}

	s.agentLock.RLock()
	defer s.agentLock.RUnlock()

	key := s.bindings[bindCode]
	if key == "" {
		return nil
	}
	return s.agents[key]
}

func (s *Server) listAgents() []AgentInfo {
	s.agentLock.RLock()
	defer s.agentLock.RUnlock()

	agents := make([]AgentInfo, 0, len(s.agents))
	for _, agent := range s.agents {
		agents = append(agents, AgentInfo{
			UserID:      agent.UserID,
			DeviceID:    agent.DeviceID,
			RemoteAddr:  agent.RemoteAddr,
			ConnectedAt: agent.ConnectedAt,
			LastSeenAt:  agent.LastSeenAt,
		})
	}

	return agents
}

func (s *Server) agentInfo(userID string, deviceID string) (AgentInfo, bool) {
	s.agentLock.RLock()
	defer s.agentLock.RUnlock()

	agent := s.agents[agentKey(userID, deviceID)]
	if agent == nil {
		return AgentInfo{}, false
	}
	return AgentInfo{
		UserID:      agent.UserID,
		DeviceID:    agent.DeviceID,
		RemoteAddr:  agent.RemoteAddr,
		ConnectedAt: agent.ConnectedAt,
		LastSeenAt:  agent.LastSeenAt,
	}, true
}

// registerClient installs a request route, or idempotently reattaches an
// existing route owned by the same paired client. A request ID cannot be
// reassigned across identities because doing so would let one client steal
// another client's in-flight response stream.
func (s *Server) registerClient(requestID string, requestType protocol.MessageType, p *peer, userID string, deviceID string, clientToken string, remoteAddr string) bool {
	if requestID == "" {
		return false
	}

	now := time.Now()

	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	s.pruneExpiredClientRoutesLocked(now)
	clientID := clientTokenHash(clientToken)
	if previous := s.clients[requestID]; previous != nil {
		if previous.UserID != userID || previous.DeviceID != deviceID || previous.ClientID != clientID {
			return false
		}
		// Same-client retries keep the original request type so an eventual
		// terminal response still releases the first route correctly.
		previous.peer = p
		previous.RemoteAddr = remoteAddr
		previous.LastSeenAt = now
		return true
	}

	s.clients[requestID] = &clientEntry{
		RequestID:   requestID,
		RequestType: requestType,
		UserID:      userID,
		DeviceID:    deviceID,
		ClientID:    clientID,
		RemoteAddr:  remoteAddr,
		ConnectedAt: now,
		LastSeenAt:  now,
		peer:        p,
	}
	return true
}

func (s *Server) touchClient(requestID string) {
	if requestID == "" {
		return
	}

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	if client := s.clients[requestID]; client != nil {
		client.LastSeenAt = time.Now()
	}
}

func (s *Server) touchClientPeer(p *peer) {
	if p == nil {
		return
	}

	now := time.Now()
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	for _, client := range s.clients {
		if client.peer == p {
			client.LastSeenAt = now
		}
	}
	if connection := s.connections[p]; connection != nil {
		connection.LastSeenAt = now
	}
}

func (s *Server) unregisterClient(requestID string) {
	if requestID == "" {
		return
	}

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	delete(s.clients, requestID)
}

func (s *Server) unregisterClientPeer(p *peer) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	for _, client := range s.clients {
		if client.peer == p {
			// Do not discard an in-flight request just because its transport
			// disappeared. A replacement connection for the same paired
			// user/device can reclaim it below.
			client.peer = nil
		}
	}
	delete(s.connections, p)
}

func (s *Server) getClient(requestID string) *clientEntry {
	if requestID == "" {
		return nil
	}

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	client := s.clients[requestID]
	if client != nil && time.Since(client.LastSeenAt) > clientRouteTTL {
		delete(s.clients, requestID)
		return nil
	}
	if client == nil {
		return nil
	}
	entry := *client
	return &entry
}

// rebindClientRequest verifies that a reconnecting client owns the request and
// attaches the request route to its current WebSocket peer.
func (s *Server) rebindClientRequest(requestID string, userID string, deviceID string, clientToken string, p *peer) bool {
	if requestID == "" || userID == "" || deviceID == "" || clientToken == "" || p == nil {
		return false
	}

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	client := s.clients[requestID]
	if client == nil || time.Since(client.LastSeenAt) > clientRouteTTL {
		if client != nil {
			delete(s.clients, requestID)
		}
		return false
	}
	if client.UserID != userID || client.DeviceID != deviceID || client.ClientID != clientTokenHash(clientToken) {
		return false
	}
	client.peer = p
	client.LastSeenAt = time.Now()
	return true
}

// clientPeerForMessage returns the live peer for a request. If the original
// peer went away, the most recently active connection for the same paired
// identity takes over the route.
func (s *Server) clientPeerForMessage(requestID string, userID string, deviceID string) (*clientEntry, *peer, bool) {
	if requestID == "" {
		return nil, nil, false
	}

	now := time.Now()
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	client := s.clients[requestID]
	if client == nil || now.Sub(client.LastSeenAt) > clientRouteTTL {
		if client != nil {
			delete(s.clients, requestID)
		}
		return nil, nil, false
	}
	if userID != "" && client.UserID != userID || deviceID != "" && client.DeviceID != deviceID {
		return nil, nil, false
	}

	target := client.peer
	if target != nil {
		connection := s.connections[target]
		if connection == nil || connection.UserID != client.UserID || connection.DeviceID != client.DeviceID || connection.ClientID != client.ClientID {
			target = nil
		}
	}
	if target == nil {
		var latest time.Time
		for candidate, connection := range s.connections {
			if connection.UserID != client.UserID || connection.DeviceID != client.DeviceID || connection.ClientID != client.ClientID {
				continue
			}
			if target == nil || connection.LastSeenAt.After(latest) {
				target = candidate
				latest = connection.LastSeenAt
			}
		}
		client.peer = target
	}
	if target == nil {
		return nil, nil, false
	}
	client.LastSeenAt = now
	// Return a value copy so the caller can safely use the route after the
	// registry lock is released.
	entry := *client
	return &entry, target, true
}

func (s *Server) registerClientConnection(p *peer, userID string, deviceID string, clientToken string, remoteAddr string) {
	if p == nil || userID == "" || deviceID == "" || clientToken == "" {
		return
	}
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	if s.connections == nil {
		s.connections = make(map[*peer]*clientConnection)
	}
	s.connections[p] = &clientConnection{
		UserID: userID, DeviceID: deviceID, ClientID: clientTokenHash(clientToken), RemoteAddr: remoteAddr, LastSeenAt: time.Now(),
	}
}

func (s *Server) pruneExpiredClientRoutesLocked(now time.Time) {
	for requestID, client := range s.clients {
		if now.Sub(client.LastSeenAt) > clientRouteTTL {
			delete(s.clients, requestID)
		}
	}
}

func (s *Server) getClientConnections(userID string, deviceID string) []*peer {
	s.clientLock.RLock()
	defer s.clientLock.RUnlock()
	result := make([]*peer, 0)
	for peer, connection := range s.connections {
		if connection.UserID == userID && connection.DeviceID == deviceID {
			result = append(result, peer)
		}
	}
	return result
}
