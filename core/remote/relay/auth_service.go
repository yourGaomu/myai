package relay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	domainauthorization "myai/core/domain/authorization"
	authorizationport "myai/core/port/authorization"
	"myai/core/remote/protocol"
)

const (
	defaultClientTokenTTL = 30 * 24 * time.Hour
	authOperationTimeout  = 5 * time.Second
)

func (s *Server) authorizeClient(userID string, deviceID string, clientName string, remoteAddr string) (string, domainauthorization.ClientAuthorization, error) {
	if userID == "" || deviceID == "" {
		return "", domainauthorization.ClientAuthorization{}, fmt.Errorf("user id or device id is empty")
	}

	token, err := newClientToken()
	if err != nil {
		return "", domainauthorization.ClientAuthorization{}, err
	}

	now := time.Now()
	authorization := domainauthorization.ClientAuthorization{
		ID:         clientTokenHash(token),
		UserID:     userID,
		DeviceID:   deviceID,
		ClientName: normalizeClientName(clientName),
		RemoteAddr: remoteAddr,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(defaultClientTokenTTL),
	}

	ctx, cancel := authContext()
	defer cancel()
	if err := s.authStore.Save(ctx, authorization); err != nil {
		return "", domainauthorization.ClientAuthorization{}, err
	}
	return token, authorization, nil
}

type clientAuthError string

const (
	clientAuthRejected    clientAuthError = "client_auth_rejected"
	clientAuthUnavailable clientAuthError = "client_auth_unavailable"
)

func (clientAuthError) Error() string { return "client token is invalid or expired" }

func (s *Server) validateClientToken(userID string, deviceID string, token string) bool {
	return s.checkClientToken(userID, deviceID, token) == nil
}

func (s *Server) checkClientToken(userID string, deviceID string, token string) error {
	if userID == "" || deviceID == "" || token == "" {
		return clientAuthRejected
	}
	if s.authStore == nil {
		return clientAuthUnavailable
	}

	ctx, cancel := authContext()
	defer cancel()
	id := clientTokenHash(token)
	authorization, err := s.authStore.Get(ctx, id)
	if err != nil {
		return classifyClientAuthStoreError(err)
	}
	now := time.Now()
	if authorization.ID != id || authorization.UserID != userID || authorization.DeviceID != deviceID || !authorization.ActiveAt(now) {
		return clientAuthRejected
	}

	return classifyClientAuthStoreError(s.authStore.Touch(ctx, id, now))
}

func classifyClientAuthStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, authorizationport.ErrNotFound):
		return clientAuthRejected
	default:
		return clientAuthUnavailable
	}
}

func (s *Server) listClientAuthorizations(userID string, deviceID string, token string) ([]AuthorizationInfo, error) {
	ctx, cancel := authContext()
	defer cancel()
	authorizations, err := s.authStore.ListActive(ctx, userID, deviceID)
	if err != nil {
		return nil, err
	}

	currentID := clientTokenHash(token)
	infos := make([]AuthorizationInfo, 0, len(authorizations))
	for _, authorization := range authorizations {
		infos = append(infos, authorizationInfoFromDomain(authorization, currentID))
	}
	return infos, nil
}

func (s *Server) revokeClientAuthorization(id string, userID string, deviceID string) error {
	ctx, cancel := authContext()
	defer cancel()
	authorization, err := s.authStore.Get(ctx, id)
	if err != nil {
		return err
	}
	if authorization.UserID != userID || authorization.DeviceID != deviceID {
		return authorizationport.ErrNotFound
	}
	if err := s.authStore.Revoke(ctx, id, time.Now()); err != nil {
		return err
	}
	s.invalidateClientAuthorization(id)
	return nil
}

// Check the durable authorization at the delivery boundary as revocations may
// originate on another relay instance. Do not extend expiry when delivering.
func (s *Server) writeAuthorizedClient(p *peer, message protocol.Message) error {
	s.clientLock.RLock()
	connection := s.connections[p]
	var identity clientConnection
	if connection != nil {
		identity = *connection
	}
	s.clientLock.RUnlock()
	if connection == nil {
		return fmt.Errorf("client connection is no longer registered")
	}
	if err := s.checkClientDelivery(identity.UserID, identity.DeviceID, identity.ClientID); err != nil {
		return err
	}
	return p.writeJSON(message)
}

func (s *Server) checkClientDelivery(userID, deviceID, clientID string) error {
	if s.authStore != nil {
		ctx, cancel := authContext()
		defer cancel()
		authorization, err := s.authStore.Get(ctx, clientID)
		if err == nil && authorization.ID == clientID && authorization.ActiveAt(time.Now()) && authorization.UserID == userID && authorization.DeviceID == deviceID {
			return nil
		}
	}
	s.invalidateClientAuthorization(clientID)
	return errors.New("client authorization is unavailable, revoked or expired")
}

// Recheck route ownership and pending state after the store lookup and after
// acquiring the socket's write lock. If a terminal response races with replay,
// its write to this socket follows a replay already in progress.
func (s *Server) writeRoutedClient(p *peer, route *clientEntry, message protocol.Message, pending *foregroundPermission) error {
	if err := s.checkClientDelivery(route.UserID, route.DeviceID, route.ClientID); err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	s.clientLock.RLock()
	ownsRoute := s.clients[route.RequestID] == route && route.ownsConnection(s.connections[p])
	if route.RequestType == protocol.TypeSessionRename {
		ownsRoute = ownsRoute && route.peer == p
	}
	if route.RequestType == protocol.TypeAssetImagePreview {
		ownsRoute = ownsRoute && route.peer == p && time.Since(route.ConnectedAt) <= assetPreviewRouteTTL
	}
	if pending != nil {
		now := time.Now()
		ownsRoute = ownsRoute && route.peer == p && now.Sub(route.LastSeenAt) <= clientRouteTTL &&
			s.foregroundPermissions[pending.key] == pending && now.Before(pending.expiresAt)
	}
	s.clientLock.RUnlock()
	if !ownsRoute {
		if pending != nil {
			return nil // A resolved or expired ask is skipped on replay.
		}
		return errors.New("client request no longer belongs to this connection")
	}
	if route.RequestType == protocol.TypeAssetImagePreview {
		if err := p.conn.SetWriteDeadline(time.Now().Add(websocketWriteWait)); err != nil {
			return err
		}
		defer p.conn.SetWriteDeadline(time.Time{})
		if err := p.conn.WriteJSON(message); err != nil {
			_ = p.close()
			return err
		}
		return nil
	}
	return p.conn.WriteJSON(message)
}

func normalizeClientName(name string) string {
	if name == "" {
		return "Browser"
	}
	if len(name) > 80 {
		return name[:80]
	}
	return name
}

func authContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), authOperationTimeout)
}

func newClientToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("create client token failed: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func clientTokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
