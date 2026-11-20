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

	foregroundFinished bool
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
		if previous.peer != p {
			superseded = previous.peer
		}
	}

	// A code keeps its first owner and original expiry, including after it has
	// been consumed or its agent reconnects. A collision never steals a code.
	if bindCode != "" {
		if binding := s.bindings[bindCode]; binding == nil {
			s.bindings[bindCode] = &bindingEntry{AgentKey: key, ExpiresAt: now.Add(bindingCodeTTL)}
		} else if binding.AgentKey != key {
			bindCode = ""
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

	binding := s.bindings[bindCode]
	if binding == nil || binding.Consumed || !time.Now().Before(binding.ExpiresAt) {
		return nil
	}
	agent := s.agents[binding.AgentKey]
	if agent == nil || agent.BindCode != bindCode {
		return nil
	}
	entry := *agent
	return &entry
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

	if client := s.clients[requestID]; client != nil {
		s.removeClientRouteLocked(client)
	}
}

// Use the original route instance, not just its request ID, after a write.
// The ID could have been reused if authorization was invalidated meanwhile.
func (s *Server) finishClientRoute(client *clientEntry) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	if s.clients[client.RequestID] == client {
		s.removeClientRouteLocked(client)
	}
}

func (s *Server) removeClientRouteLocked(client *clientEntry) {
	delete(s.clients, client.RequestID)
	s.clearForegroundPermissionsLocked(client)
}

func (s *Server) unregisterClientPeer(p *peer) {
	s.unregisterClientPeerForRoute(p, nil)
}

func (s *Server) unregisterClientPeerForRoute(p *peer, owner *clientEntry) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	if owner != nil && !owner.ownsConnection(s.connections[p]) {
		return
	}

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

func (s *Server) invalidateClientAuthorization(clientID string) {
	var peers []*peer
	s.clientLock.Lock()
	for _, client := range s.clients {
		if client.ClientID == clientID {
			s.removeClientRouteLocked(client)
		}
	}
	for p, connection := range s.connections {
		if connection.ClientID == clientID {
			delete(s.connections, p)
			peers = append(peers, p)
		}
	}
	s.clientLock.Unlock()
	for _, p := range peers {
		_ = p.close()
	}
}

func (s *Server) getClient(requestID string) *clientEntry {
	if requestID == "" {
		return nil
	}

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	client := s.clients[requestID]
	if client != nil && time.Since(client.LastSeenAt) > clientRouteTTL {
		s.removeClientRouteLocked(client)
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
			s.removeClientRouteLocked(client)
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

// clientPeerForMessage correlates an agent response with the original request.
// On retry, require the same route even if the request ID has since been reused.
func (s *Server) clientPeerForMessage(message protocol.Message, expected *clientEntry) (*clientEntry, *peer, bool) {
	if message.RequestID == "" {
		return nil, nil, false
	}

	now := time.Now()
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	client := s.clients[message.RequestID]
	if expected != nil && client != expected {
		return nil, nil, false
	}
	if client == nil || now.Sub(client.LastSeenAt) > clientRouteTTL {
		if client != nil {
			s.removeClientRouteLocked(client)
		}
		return nil, nil, false
	}
	if message.UserID != "" && client.UserID != message.UserID || message.DeviceID != "" && client.DeviceID != message.DeviceID {
		return nil, nil, false
	}

	terminal := isTerminalClientResponse(client.RequestType, message)
	s.trackForegroundPermissionLocked(client, message, terminal, now)
	target := client.peer
	if target != nil && !client.ownsConnection(s.connections[target]) {
		target = nil
	}
	if target == nil {
		var latest time.Time
		for candidate, connection := range s.connections {
			if !client.ownsConnection(connection) {
				continue
			}
			if target == nil || connection.LastSeenAt.After(latest) {
				target = candidate
				latest = connection.LastSeenAt
			}
		}
		client.peer = target
	}
	if target != nil {
		client.LastSeenAt = now
	}
	return client, target, terminal
}

func (client *clientEntry) ownsConnection(connection *clientConnection) bool {
	return connection != nil && connection.ClientID == client.ClientID && connection.UserID == client.UserID && connection.DeviceID == client.DeviceID
}

func (s *Server) registerClientConnection(p *peer, userID string, deviceID string, clientToken string, remoteAddr string) {
	if p == nil || userID == "" || deviceID == "" || clientToken == "" {
		return
	}
	// Serialize identity changes with writes that validate the socket owner.
	p.writeMu.Lock()
	s.clientLock.Lock()
	if s.connections == nil {
		s.connections = make(map[*peer]*clientConnection)
	}
	newConnection := s.connections[p] == nil
	identity := &clientConnection{
		UserID: userID, DeviceID: deviceID, ClientID: clientTokenHash(clientToken), RemoteAddr: remoteAddr, LastSeenAt: time.Now(),
	}
	s.connections[p] = identity
	var foreground []*foregroundPermission
	if newConnection {
		foreground = s.reattachForegroundPermissionsLocked(p, identity)
	}
	s.clientLock.Unlock()
	p.writeMu.Unlock()
	if newConnection {
		s.replayInteractions(p, userID, deviceID)
		s.replayForegroundPermissions(p, foreground)
	}
}

func (s *Server) pruneExpiredClientRoutesLocked(now time.Time) {
	for _, client := range s.clients {
		if now.Sub(client.LastSeenAt) > clientRouteTTL {
			s.removeClientRouteLocked(client)
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
