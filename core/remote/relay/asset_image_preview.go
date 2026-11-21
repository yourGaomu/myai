package relay

import (
	"errors"
	"time"

	"myai/core/remote/protocol"
)

const assetPreviewRouteTTL = 30 * time.Second
const maxAssetPreviewsPerClient = 8

func (s *Server) routeAssetImagePreview(p *peer, remoteAddr string, message protocol.Message) error {
	payload, err := protocol.AssetImagePreviewRequest(message)
	if err != nil {
		return err
	}
	if err := s.checkClientToken(message.UserID, message.DeviceID, message.ClientToken); err != nil {
		return err
	}
	route, err := s.registerAssetPreview(p, remoteAddr, message, payload.AssetID)
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

func (s *Server) registerAssetPreview(p *peer, remoteAddr string, message protocol.Message, assetID string) (*clientEntry, error) {
	now := time.Now()
	clientID := clientTokenHash(message.ClientToken)
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	s.pruneExpiredClientRoutesLocked(now)
	if s.clients[message.RequestID] != nil {
		return nil, errors.New("image preview request ID is already in use")
	}
	active := 0
	for _, route := range s.clients {
		if route.RequestType == protocol.TypeAssetImagePreview && route.ClientID == clientID && route.peer != nil {
			active++
		}
	}
	if active >= maxAssetPreviewsPerClient {
		return nil, errors.New("too many pending image previews; retry shortly")
	}
	route := &clientEntry{
		RequestID: message.RequestID, RequestType: message.Type, UserID: message.UserID, DeviceID: message.DeviceID,
		ClientID: clientID, RemoteAddr: remoteAddr, ConnectedAt: now, LastSeenAt: now, peer: p,
		previewSessionID: message.SessionID, previewAssetID: assetID,
	}
	s.clients[message.RequestID] = route
	return route, nil
}

func (s *Server) forwardAssetImagePreview(message protocol.Message) error {
	var assetID string
	switch message.Type {
	case protocol.TypeAssetImagePreviewResult:
		payload, err := protocol.AssetImagePreviewResult(message)
		if err != nil {
			return err
		}
		assetID = payload.AssetID
	case protocol.TypeError:
		if controlReplyID(message, "request_type") != string(protocol.TypeAssetImagePreview) {
			return errors.New("image preview error does not match its request type")
		}
	default:
		return errors.New("invalid image preview response")
	}

	s.clientLock.RLock()
	route := s.clients[message.RequestID]
	var target *peer
	if route != nil && route.RequestType == protocol.TypeAssetImagePreview && route.UserID == message.UserID &&
		route.DeviceID == message.DeviceID && route.previewSessionID == message.SessionID &&
		(assetID == "" || route.previewAssetID == assetID) && time.Since(route.ConnectedAt) <= assetPreviewRouteTTL &&
		route.ownsConnection(s.connections[route.peer]) {
		target = route.peer
	}
	s.clientLock.RUnlock()
	if target == nil {
		// The requester closed or switched its connection, or this is a stale or
		// unrelated response. Preview bytes are never replayed on another socket.
		return nil
	}
	defer s.finishClientRoute(route)
	return s.writeRoutedClient(target, route, message, nil)
}
