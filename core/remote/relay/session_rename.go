package relay

import (
	"errors"
	"time"

	"myai/core/remote/protocol"
)

func (s *Server) routeSessionRename(p *peer, remoteAddr string, message protocol.Message) error {
	if err := s.checkClientToken(message.UserID, message.DeviceID, message.ClientToken); err != nil {
		return err
	}
	payload, err := protocol.SessionRenameRequest(message)
	if err != nil {
		return err
	}
	// Canonicalize the envelope so Agent errors also carry the target session ID.
	message.SessionID = payload.SessionID
	route, err := s.registerSessionRename(p, remoteAddr, message)
	if err != nil {
		return err
	}
	s.registerClientConnection(p, message.UserID, message.DeviceID, message.ClientToken, remoteAddr)
	if err := s.forwardToAgent(message); err != nil {
		s.finishClientRoute(route)
		return err
	}
	return nil
}

func (s *Server) registerSessionRename(p *peer, remoteAddr string, message protocol.Message) (*clientEntry, error) {
	now := time.Now()
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	s.pruneExpiredClientRoutesLocked(now)
	if s.clients[message.RequestID] != nil {
		return nil, errors.New("session rename request id is already in use")
	}
	route := &clientEntry{
		RequestID: message.RequestID, RequestType: message.Type, UserID: message.UserID, DeviceID: message.DeviceID,
		ClientID: clientTokenHash(message.ClientToken), RemoteAddr: remoteAddr, ConnectedAt: now, LastSeenAt: now,
		peer: p, renameSessionID: message.SessionID,
	}
	s.clients[message.RequestID] = route
	return route, nil
}

func (s *Server) forwardSessionRename(message protocol.Message) error {
	if message.Type == protocol.TypeSessionRenameResult {
		result, err := protocol.DecodePayload[protocol.SessionRenameResultPayload](message)
		if err != nil || result.SessionID != message.SessionID || result.TitleRevision <= 0 || result.Title == "" {
			return nil
		}
	}

	s.clientLock.RLock()
	route := s.clients[message.RequestID]
	if route == nil || route.RequestType != protocol.TypeSessionRename || route.UserID != message.UserID ||
		route.DeviceID != message.DeviceID || route.renameSessionID != message.SessionID || time.Since(route.LastSeenAt) > clientRouteTTL {
		s.clientLock.RUnlock()
		return nil
	}
	target := route.peer
	if !route.ownsConnection(s.connections[target]) {
		target = nil
	}
	s.clientLock.RUnlock()
	defer s.finishClientRoute(route)
	// A reconnect reads the persisted title from the session list.
	if target == nil {
		return nil
	}
	return s.writeRoutedClient(target, route, message, nil)
}
