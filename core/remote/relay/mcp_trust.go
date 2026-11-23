package relay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
	"myai/core/trust"
)

const (
	mcpTrustIDPrefix      = "relay-mcp-trust-"
	mcpTrustRouteTTL      = 30 * time.Second
	maxMCPTrustPerPairing = 4
	maxMCPTrustRoutes     = 256
	maxMCPTrustCheckBody  = 128
)

var errMCPTrustUnavailable = errors.New("mcp trust is unavailable")

type mcpTrustRoute struct {
	agentID      string
	agent        *peer
	connectionID string
	sessionID    string
	request      protocol.MCPTrustRequest
	checked      bool // guarded by clientLock; successful approve checks are single-use
	settling     bool // guarded by clientLock
}

func (s *Server) routeMCPTrust(client *peer, remoteAddr string, message protocol.Message) error {
	var request protocol.MCPTrustRequest
	if len(message.Payload) > protocol.MCPTrustMaxRequestBytes || json.Unmarshal(message.Payload, &request) != nil || request.Validate() != nil || strings.TrimSpace(message.RequestID) == "" {
		return errors.New("invalid mcp trust request")
	}
	deny := func(code string) error {
		response, err := protocol.NewMessage(protocol.TypeMCPTrustResult, message.RequestID, message.UserID, message.DeviceID, message.SessionID,
			protocol.MCPTrustResult{Action: request.Action, ServerName: request.ServerName, Code: code})
		if err != nil {
			return errMCPTrustUnavailable
		}
		return client.writeJSON(response)
	}
	if !client.mcpTrustTransport {
		return deny(protocol.MCPTrustUnavailable)
	}
	if err := s.checkClientToken(message.UserID, message.DeviceID, message.ClientToken); err != nil {
		if errors.Is(err, clientAuthRejected) {
			return deny(protocol.MCPTrustDenied)
		}
		return deny(protocol.MCPTrustUnavailable)
	}
	pairingID := clientTokenHash(message.ClientToken)
	s.registerClientConnection(client, message.UserID, message.DeviceID, message.ClientToken, remoteAddr)
	route := s.registerMCPTrust(client, message, pairingID, request)
	if route == nil {
		return deny(protocol.MCPTrustUnavailable)
	}

	// Build a fresh message: no client credentials, asserted identity fields
	// from the payload, or Mobile correlation ID cross the Agent boundary.
	internal, err := protocol.NewMessage(protocol.TypeMCPTrustInternal, route.mcpTrust.agentID, route.UserID, route.DeviceID, route.mcpTrust.sessionID,
		protocol.MCPTrustInternal{Request: request, PairingID: pairingID, ConnectionID: route.mcpTrust.connectionID})
	if err == nil {
		err = s.writeMCPTrustRequest(route, internal)
	}
	if err != nil {
		s.finishClientRoute(route)
		return deny(protocol.MCPTrustUnavailable)
	}
	return nil
}

func (s *Server) registerMCPTrust(client *peer, message protocol.Message, pairingID string, request protocol.MCPTrustRequest) *clientEntry {
	s.agentLock.RLock()
	defer s.agentLock.RUnlock()
	agent := s.agents[agentKey(message.UserID, message.DeviceID)]
	if agent == nil || !agent.peer.mcpTrustTransport {
		return nil
	}
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	now := time.Now()
	s.pruneExpiredClientRoutesLocked(now)
	if s.clients[message.RequestID] != nil || len(s.mcpTrustAgentRoutes) >= maxMCPTrustRoutes {
		return nil
	}
	connection := s.connections[client]
	if connection == nil || connection.UserID != message.UserID || connection.DeviceID != message.DeviceID || connection.ClientID != pairingID {
		return nil
	}
	active := 0
	for _, route := range s.mcpTrustAgentRoutes {
		if route.ClientID == pairingID {
			active++
		}
	}
	if active >= maxMCPTrustPerPairing {
		return nil
	}
	if connection.mcpTrustID == "" {
		connection.mcpTrustID = uuid.NewString()
	}
	if s.mcpTrustAgentRoutes == nil {
		s.mcpTrustAgentRoutes = make(map[string]*clientEntry)
	}
	route := &clientEntry{
		RequestID: message.RequestID, RequestType: protocol.TypeMCPTrust, UserID: message.UserID, DeviceID: message.DeviceID,
		ClientID: pairingID, RemoteAddr: connection.RemoteAddr, ConnectedAt: now, LastSeenAt: now, peer: client,
		mcpTrust: &mcpTrustRoute{agentID: mcpTrustIDPrefix + uuid.NewString(), agent: agent.peer,
			connectionID: connection.mcpTrustID, sessionID: message.SessionID, request: request},
	}
	s.clients[route.RequestID] = route
	s.mcpTrustAgentRoutes[route.mcpTrust.agentID] = route
	return route
}

// Call only under agentLock; trust routing always acquires agentLock before clientLock.
func (s *Server) removeMCPTrustAgentRoutes(agent *peer) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	for _, route := range s.mcpTrustAgentRoutes {
		if route.mcpTrust.agent == agent {
			s.removeClientRouteLocked(route)
		}
	}
}

// Call with both agentLock and clientLock held, in that order.
func (s *Server) mcpTrustRouteActiveLocked(route *clientEntry) bool {
	if route == nil || route.expired(time.Now()) || s.clients[route.RequestID] != route || s.mcpTrustAgentRoutes[route.mcpTrust.agentID] != route {
		return false
	}
	agent := s.agents[agentKey(route.UserID, route.DeviceID)]
	conn := s.connections[route.peer]
	return agent != nil && agent.peer == route.mcpTrust.agent && agent.peer.mcpTrustTransport &&
		route.ownsConnection(conn) && conn.mcpTrustID == route.mcpTrust.connectionID && route.peer.mcpTrustTransport
}

func (s *Server) mcpTrustRouteActive(route *clientEntry) bool {
	s.agentLock.RLock()
	defer s.agentLock.RUnlock()
	s.clientLock.RLock()
	defer s.clientLock.RUnlock()
	return s.mcpTrustRouteActiveLocked(route)
}

func (s *Server) writeMCPTrustRequest(route *clientEntry, message protocol.Message) error {
	encoded, err := json.Marshal(message)
	if err != nil || len(encoded) > protocol.MCPTrustMaxRequestBytes {
		return errMCPTrustUnavailable
	}
	agent := route.mcpTrust.agent
	agent.writeMu.Lock()
	defer agent.writeMu.Unlock()
	if !s.mcpTrustRouteActive(route) {
		return errMCPTrustUnavailable
	}
	return route.writeMCPTrustFrame(agent, encoded)
}

// The caller holds socket.writeMu. Do not hold registry locks across network writes.
func (route *clientEntry) writeMCPTrustFrame(socket *peer, encoded []byte) error {
	deadline := route.ConnectedAt.Add(mcpTrustRouteTTL)
	if now := time.Now(); !now.Before(deadline) {
		return errMCPTrustUnavailable
	} else if writeDeadline := now.Add(websocketWriteWait); writeDeadline.Before(deadline) {
		deadline = writeDeadline
	}
	if err := socket.conn.SetWriteDeadline(deadline); err != nil {
		return errMCPTrustUnavailable
	}
	defer socket.conn.SetWriteDeadline(time.Time{})
	if err := socket.conn.WriteMessage(websocket.TextMessage, encoded); err != nil {
		_ = socket.close()
		return errMCPTrustUnavailable
	}
	return nil
}

func (s *Server) handleMCPTrustCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	identity, authenticated := s.authenticateAgentRequest(r)
	if !authenticated || !s.mcpTrustTransportAllowed(r) {
		http.Error(w, "agent authentication on a secure transport required", http.StatusUnauthorized)
		return
	}
	var check protocol.MCPTrustCheck
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMCPTrustCheckBody))
	decoder.DisallowUnknownFields()
	if r.URL.RawQuery != "" || decoder.Decode(&check) != nil || decoder.Decode(new(any)) != io.EOF || !validMCPTrustAgentID(check.RequestID) {
		http.Error(w, "invalid mcp trust check", http.StatusBadRequest)
		return
	}
	if !s.checkMCPTrustRoute(r.Context(), identity, check.RequestID) {
		http.Error(w, "mcp trust check denied", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validMCPTrustAgentID(id string) bool {
	suffix, ok := strings.CutPrefix(id, mcpTrustIDPrefix)
	if !ok || len(suffix) != 36 {
		return false
	}
	value, err := uuid.Parse(suffix)
	return err == nil && value.String() == suffix
}

func (s *Server) checkMCPTrustRoute(ctx context.Context, identity agentIdentity, requestID string) bool {
	s.agentLock.RLock()
	s.clientLock.RLock()
	route := s.mcpTrustAgentRoutes[requestID]
	active := route != nil && agentKey(route.UserID, route.DeviceID) == identity && s.mcpTrustRouteActiveLocked(route) &&
		!route.mcpTrust.settling && !(route.mcpTrust.request.Action == "approve" && route.mcpTrust.checked)
	s.clientLock.RUnlock()
	s.agentLock.RUnlock()
	if !active {
		return false
	}

	// The private ID selects the exact session and both sockets. Re-read the
	// pairing from the store (including revocations on another Relay), then
	// check those same sockets again before granting or consuming the check.
	expiresAt, ok := s.mcpTrustPairingExpiry(ctx, route)
	if !ok {
		return false
	}
	s.agentLock.RLock()
	defer s.agentLock.RUnlock()
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	if ctx.Err() != nil || !time.Now().Before(expiresAt) || !s.mcpTrustRouteActiveLocked(route) || route.mcpTrust.settling ||
		(route.mcpTrust.request.Action == "approve" && route.mcpTrust.checked) {
		return false
	}
	route.mcpTrust.checked = true
	return true
}

func (s *Server) mcpTrustPairingExpiry(ctx context.Context, route *clientEntry) (time.Time, bool) {
	if s.authStore == nil {
		return time.Time{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, authOperationTimeout)
	defer cancel()
	pairing, err := s.authStore.Get(ctx, route.ClientID)
	return pairing.ExpiresAt, err == nil && ctx.Err() == nil && pairing.ID == route.ClientID && pairing.UserID == route.UserID &&
		pairing.DeviceID == route.DeviceID && pairing.ActiveAt(time.Now())
}

func (s *Server) forwardMCPTrust(agent *peer, message protocol.Message) error {
	s.agentLock.RLock()
	s.clientLock.Lock()
	route := s.mcpTrustAgentRoutes[message.RequestID]
	if route == nil || agent != route.mcpTrust.agent || route.UserID != message.UserID || route.DeviceID != message.DeviceID ||
		route.mcpTrust.sessionID != message.SessionID || !s.mcpTrustRouteActiveLocked(route) || route.mcpTrust.settling {
		s.clientLock.Unlock()
		s.agentLock.RUnlock()
		return nil
	}
	checked := route.mcpTrust.checked
	request := route.mcpTrust.request
	s.clientLock.Unlock()
	s.agentLock.RUnlock()

	result := protocol.MCPTrustResult{Action: request.Action, ServerName: request.ServerName, Code: protocol.MCPTrustUnavailable}
	if message.Type == protocol.TypeMCPTrustResult && len(message.Payload) <= protocol.MCPTrustMaxResponseBytes {
		var candidate protocol.MCPTrustResult
		if json.Unmarshal(message.Payload, &candidate) == nil {
			if candidate.Action != request.Action || candidate.ServerName != request.ServerName {
				return nil // A mismatched result cannot settle this route.
			}
			switch candidate.Code {
			case protocol.MCPTrustDenied, protocol.MCPTrustReviewRequired, protocol.MCPTrustUnavailable:
				result.Code = candidate.Code // Never forward denial details or service payloads.
			case "":
				if checked && validMCPTrustResult(request, candidate) {
					result = candidate
				}
			}
		}
	}
	// Legacy errors and malformed/oversized responses become a fixed failure.
	response, err := protocol.NewMessage(protocol.TypeMCPTrustResult, route.RequestID, route.UserID, route.DeviceID, route.mcpTrust.sessionID, result)
	if err != nil {
		return errMCPTrustUnavailable
	}
	encoded, err := json.Marshal(response)
	if err != nil || len(encoded) > protocol.MCPTrustMaxResponseBytes {
		response, err = protocol.NewMessage(protocol.TypeMCPTrustResult, route.RequestID, route.UserID, route.DeviceID, route.mcpTrust.sessionID,
			protocol.MCPTrustResult{Action: request.Action, ServerName: request.ServerName, Code: protocol.MCPTrustUnavailable})
		if err != nil {
			return errMCPTrustUnavailable
		}
		encoded, err = json.Marshal(response)
		if err != nil || len(encoded) > protocol.MCPTrustMaxResponseBytes {
			return errMCPTrustUnavailable
		}
	}

	s.agentLock.RLock()
	s.clientLock.Lock()
	if !s.mcpTrustRouteActiveLocked(route) || route.mcpTrust.settling {
		s.clientLock.Unlock()
		s.agentLock.RUnlock()
		return nil
	}
	route.mcpTrust.settling = true
	s.clientLock.Unlock()
	s.agentLock.RUnlock()
	defer s.finishClientRoute(route)

	expiresAt, ok := s.mcpTrustPairingExpiry(context.Background(), route)
	if !ok {
		return nil
	}
	client := route.peer // Trust routes never change sockets.
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	if !s.mcpTrustRouteActive(route) || !time.Now().Before(expiresAt) {
		return nil
	}
	return route.writeMCPTrustFrame(client, encoded)
}

func validMCPTrustResult(request protocol.MCPTrustRequest, result protocol.MCPTrustResult) bool {
	if len(result.Services) > protocol.MCPTrustMaxServices {
		return false
	}
	switch request.Action {
	case "list":
		if result.ReviewID != "" {
			return false
		}
	case "review":
		approval := protocol.MCPTrustRequest{Action: "approve", ServerName: request.ServerName, ReviewID: result.ReviewID}
		if approval.Validate() != nil || len(result.Services) != 1 || result.Services[0].Name != request.ServerName {
			return false
		}
	case "approve":
		if result.ReviewID != "" || len(result.Services) != 1 || result.Services[0].Name != request.ServerName || !result.Services[0].Trusted {
			return false
		}
	default:
		return false
	}
	seen := make(map[string]struct{}, len(result.Services))
	for _, service := range result.Services {
		if !protocol.ValidMCPTrustServerName(service.Name) || !trust.SafeMobileMCPEndpoint(service.Endpoint) {
			return false
		}
		if _, duplicate := seen[service.Name]; duplicate {
			return false
		}
		seen[service.Name] = struct{}{}
		switch service.Permission {
		case "read", "write", "execute":
		default:
			return false
		}
		switch service.Authentication {
		case "none", "headers_configured":
		default:
			return false
		}
		switch service.Runtime {
		case "loaded", "older", "not_loaded":
		default:
			return false
		}
	}
	return true
}
