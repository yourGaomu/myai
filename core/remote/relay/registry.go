package relay

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

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
	UserID      string
	DeviceID    string
	RemoteAddr  string
	ConnectedAt time.Time
	LastSeenAt  time.Time
	peer        *peer
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

func (s *Server) registerAgent(p *peer, userID string, deviceID string, bindCode string, remoteAddr string) {
	if userID == "" || deviceID == "" {
		return
	}

	now := time.Now()
	key := agentKey(userID, deviceID)

	s.agentLock.Lock()
	defer s.agentLock.Unlock()

	if previous := s.agents[key]; previous != nil && previous.BindCode != "" {
		delete(s.bindings, previous.BindCode)
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

func (s *Server) registerClient(requestID string, p *peer, userID string, deviceID string, remoteAddr string) {
	if requestID == "" {
		return
	}

	now := time.Now()

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	s.clients[requestID] = &clientEntry{
		RequestID:   requestID,
		UserID:      userID,
		DeviceID:    deviceID,
		RemoteAddr:  remoteAddr,
		ConnectedAt: now,
		LastSeenAt:  now,
		peer:        p,
	}
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

	for requestID, client := range s.clients {
		if client.peer == p {
			delete(s.clients, requestID)
		}
	}
}

func (s *Server) getClient(requestID string) *clientEntry {
	if requestID == "" {
		return nil
	}

	s.clientLock.RLock()
	defer s.clientLock.RUnlock()

	return s.clients[requestID]
}
