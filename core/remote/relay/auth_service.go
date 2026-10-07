package relay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

func (s *Server) validateClientToken(userID string, deviceID string, token string) bool {
	if userID == "" || deviceID == "" || token == "" {
		return false
	}

	ctx, cancel := authContext()
	defer cancel()
	authorization, err := s.authStore.Get(ctx, clientTokenHash(token))
	if err != nil {
		return false
	}
	if authorization.UserID != userID || authorization.DeviceID != deviceID {
		return false
	}
	if !authorization.ActiveAt(time.Now()) {
		return false
	}

	if err := s.authStore.Touch(ctx, authorization.ID, time.Now()); err != nil {
		return false
	}
	return true
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
	ctx, cancel := authContext()
	defer cancel()
	authorization, err := s.authStore.Get(ctx, identity.ClientID)
	if err != nil || !authorization.ActiveAt(time.Now()) || authorization.UserID != identity.UserID || authorization.DeviceID != identity.DeviceID {
		s.invalidateClientAuthorization(identity.ClientID)
		return fmt.Errorf("client authorization is unavailable, revoked or expired")
	}
	return p.writeJSON(message)
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
