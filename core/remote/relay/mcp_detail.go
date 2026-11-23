package relay

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

const (
	mcpDetailRouteTTL      = 30 * time.Second
	maxMCPDetailsPerClient = 8
	mcpDetailIDPrefix      = "relay-mcp-detail-"
)

func (s *Server) routeMCPDetail(p *peer, remoteAddr string, message protocol.Message) error {
	payload, err := protocol.MCPDetailRequest(message)
	if err != nil {
		return err
	}
	if err := s.checkClientToken(message.UserID, message.DeviceID, message.ClientToken); err != nil {
		return err
	}
	agent := s.getAgent(message.UserID, message.DeviceID)
	if agent == nil {
		return errors.New("agent is not online")
	}
	route, err := s.registerMCPDetail(p, agent.peer, remoteAddr, message, payload.ServerName)
	if err != nil {
		return err
	}
	s.registerClientConnection(p, message.UserID, message.DeviceID, message.ClientToken, remoteAddr)
	// A late reply can never settle a new request that reuses the Mobile ID,
	// including after the first route expires or another paired client reuses it.
	message.RequestID = route.detailAgentID
	encoded, err := json.Marshal(message)
	if err != nil || len(encoded) > protocol.MCPDetailRequestMaxJSONBytes {
		s.finishClientRoute(route)
		return errors.New("mcp detail request is too large")
	}
	if err := route.writeMCPDetailRequest(encoded); err != nil {
		s.finishClientRoute(route)
		return err
	}
	return nil
}

func (route *clientEntry) mcpDetailWriteDeadline(now time.Time) time.Time {
	deadline := route.ConnectedAt.Add(mcpDetailRouteTTL)
	if writeDeadline := now.Add(websocketWriteWait); writeDeadline.Before(deadline) {
		return writeDeadline
	}
	return deadline
}

func (route *clientEntry) writeMCPDetailRequest(encoded []byte) error {
	agent := route.detailAgent
	agent.writeMu.Lock()
	defer agent.writeMu.Unlock()
	now := time.Now()
	if route.expired(now) {
		return errors.New("mcp detail request expired before forwarding")
	}
	if err := agent.conn.SetWriteDeadline(route.mcpDetailWriteDeadline(now)); err != nil {
		return errors.New("could not forward mcp detail request")
	}
	defer agent.conn.SetWriteDeadline(time.Time{})
	if err := agent.conn.WriteMessage(websocket.TextMessage, encoded); err != nil {
		_ = agent.close()
		return errors.New("could not forward mcp detail request")
	}
	return nil
}

func (s *Server) registerMCPDetail(client, agent *peer, remoteAddr string, message protocol.Message, serverName string) (*clientEntry, error) {
	now := time.Now()
	clientID := clientTokenHash(message.ClientToken)
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	s.pruneExpiredClientRoutesLocked(now)
	if s.clients[message.RequestID] != nil {
		return nil, errors.New("mcp detail request ID is already in use")
	}
	active := 0
	for _, route := range s.mcpDetailAgentRoutes {
		if route.ClientID == clientID {
			active++
		}
	}
	if active >= maxMCPDetailsPerClient {
		return nil, errors.New("too many pending mcp detail requests; retry shortly")
	}
	if s.mcpDetailAgentRoutes == nil {
		s.mcpDetailAgentRoutes = make(map[string]*clientEntry)
	}
	route := &clientEntry{
		RequestID: message.RequestID, RequestType: message.Type, UserID: message.UserID, DeviceID: message.DeviceID,
		ClientID: clientID, RemoteAddr: remoteAddr, ConnectedAt: now, LastSeenAt: now, peer: client,
		detailSessionID: message.SessionID, detailServerName: serverName, detailAgent: agent,
		detailAgentID: mcpDetailIDPrefix + uuid.NewString(),
	}
	s.clients[route.RequestID] = route
	s.mcpDetailAgentRoutes[route.detailAgentID] = route
	return route, nil
}

func (s *Server) forwardMCPDetail(agent *peer, message protocol.Message) error {
	if !strings.HasPrefix(message.RequestID, mcpDetailIDPrefix) {
		return nil
	}
	var serverName string
	var result any
	switch message.Type {
	case protocol.TypeMCPDetailResult:
		payload, err := protocol.MCPDetailResult(message)
		if err != nil {
			return err
		}
		serverName, result = payload.Server.Name, payload
	case protocol.TypeError:
		payload, err := protocol.MCPDetailError(message)
		if err != nil {
			return err
		}
		serverName, result = payload.ServerName, payload
	default:
		return errors.New("invalid mcp detail response type")
	}

	s.clientLock.Lock()
	route := s.mcpDetailAgentRoutes[message.RequestID]
	if route != nil && route.expired(time.Now()) {
		s.removeClientRouteLocked(route)
		route = nil
	}
	if route == nil || route.detailAgent != agent || route.UserID != message.UserID || route.DeviceID != message.DeviceID ||
		route.detailSessionID != message.SessionID || serverName != "" && route.detailServerName != serverName {
		s.clientLock.Unlock()
		return nil
	}
	target := route.peer
	if !route.ownsConnection(s.connections[target]) {
		target = nil
	}
	s.clientLock.Unlock()
	if target == nil {
		// Never deliver an inspection to a replacement Mobile socket.
		s.finishClientRoute(route)
		return nil
	}

	// Forward only the validated projection and restore the Mobile ID. Check
	// canonical encoding too, since escaping may make it larger than the input.
	response, err := protocol.NewMessage(message.Type, route.RequestID, message.UserID, message.DeviceID, message.SessionID, result)
	if err != nil || len(response.Payload) > protocol.MCPDetailResponseMaxBytes {
		return errors.New("mcp detail response is too large")
	}
	defer s.finishClientRoute(route)
	return s.writeRoutedClient(target, route, response, nil)
}
