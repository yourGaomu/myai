package relay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	defaultClientTokenTTL = 30 * 24 * time.Hour
	authOperationTimeout  = 5 * time.Second
)

var errAuthorizationNotFound = errors.New("authorization not found")

type AuthStore interface {
	SaveAuthorization(ctx context.Context, authorization ClientAuthorization) error
	GetAuthorization(ctx context.Context, id string) (ClientAuthorization, error)
	TouchAuthorization(ctx context.Context, id string, lastSeenAt time.Time) error
	RevokeAuthorization(ctx context.Context, id string, revokedAt time.Time) error
	ListAuthorizations(ctx context.Context, userID string, deviceID string) ([]ClientAuthorization, error)
}

type ClientAuthorization struct {
	ID         string     `bson:"_id" json:"id"`
	UserID     string     `bson:"user_id" json:"user_id"`
	DeviceID   string     `bson:"device_id" json:"device_id"`
	ClientName string     `bson:"client_name" json:"client_name"`
	RemoteAddr string     `bson:"remote_addr" json:"remote_addr"`
	CreatedAt  time.Time  `bson:"created_at" json:"created_at"`
	LastSeenAt time.Time  `bson:"last_seen_at" json:"last_seen_at"`
	ExpiresAt  time.Time  `bson:"expires_at" json:"expires_at"`
	RevokedAt  *time.Time `bson:"revoked_at,omitempty" json:"revoked_at,omitempty"`
}

type AuthorizationInfo struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	DeviceID   string     `json:"device_id"`
	ClientName string     `json:"client_name"`
	RemoteAddr string     `json:"remote_addr"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	Current    bool       `json:"current"`
}

type MemoryAuthStore struct {
	mu             sync.RWMutex
	authorizations map[string]ClientAuthorization
}

func NewMemoryAuthStore() *MemoryAuthStore {
	return &MemoryAuthStore{
		authorizations: make(map[string]ClientAuthorization),
	}
}

func (s *MemoryAuthStore) SaveAuthorization(ctx context.Context, authorization ClientAuthorization) error {
	if authorization.ID == "" {
		return errors.New("authorization id is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.authorizations[authorization.ID] = authorization
	return nil
}

func (s *MemoryAuthStore) GetAuthorization(ctx context.Context, id string) (ClientAuthorization, error) {
	if id == "" {
		return ClientAuthorization{}, errAuthorizationNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	authorization, ok := s.authorizations[id]
	if !ok {
		return ClientAuthorization{}, errAuthorizationNotFound
	}
	return authorization, nil
}

func (s *MemoryAuthStore) TouchAuthorization(ctx context.Context, id string, lastSeenAt time.Time) error {
	if id == "" {
		return errAuthorizationNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	authorization, ok := s.authorizations[id]
	if !ok {
		return errAuthorizationNotFound
	}
	authorization.LastSeenAt = lastSeenAt
	s.authorizations[id] = authorization
	return nil
}

func (s *MemoryAuthStore) RevokeAuthorization(ctx context.Context, id string, revokedAt time.Time) error {
	if id == "" {
		return errAuthorizationNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	authorization, ok := s.authorizations[id]
	if !ok {
		return errAuthorizationNotFound
	}
	authorization.RevokedAt = &revokedAt
	s.authorizations[id] = authorization
	return nil
}

func (s *MemoryAuthStore) ListAuthorizations(ctx context.Context, userID string, deviceID string) ([]ClientAuthorization, error) {
	now := time.Now()

	s.mu.RLock()
	defer s.mu.RUnlock()

	authorizations := make([]ClientAuthorization, 0)
	for _, authorization := range s.authorizations {
		if authorization.UserID != userID || authorization.DeviceID != deviceID {
			continue
		}
		if !authorization.isActive(now) {
			continue
		}
		authorizations = append(authorizations, authorization)
	}

	sortAuthorizations(authorizations)
	return authorizations, nil
}

func (s *Server) authorizeClient(userID string, deviceID string, clientName string, remoteAddr string) (string, ClientAuthorization, error) {
	if userID == "" || deviceID == "" {
		return "", ClientAuthorization{}, fmt.Errorf("user id or device id is empty")
	}

	token, err := newClientToken()
	if err != nil {
		return "", ClientAuthorization{}, err
	}

	now := time.Now()
	authorization := ClientAuthorization{
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

	if err := s.authStore.SaveAuthorization(ctx, authorization); err != nil {
		return "", ClientAuthorization{}, err
	}
	return token, authorization, nil
}

func (s *Server) validateClientToken(userID string, deviceID string, token string) bool {
	if userID == "" || deviceID == "" || token == "" {
		return false
	}

	ctx, cancel := authContext()
	defer cancel()

	authorization, err := s.authStore.GetAuthorization(ctx, clientTokenHash(token))
	if err != nil {
		return false
	}
	if authorization.UserID != userID || authorization.DeviceID != deviceID {
		return false
	}
	if !authorization.isActive(time.Now()) {
		return false
	}

	if err := s.authStore.TouchAuthorization(ctx, authorization.ID, time.Now()); err != nil {
		return false
	}
	return true
}

func (s *Server) listClientAuthorizations(userID string, deviceID string, token string) ([]AuthorizationInfo, error) {
	ctx, cancel := authContext()
	defer cancel()

	authorizations, err := s.authStore.ListAuthorizations(ctx, userID, deviceID)
	if err != nil {
		return nil, err
	}

	currentID := clientTokenHash(token)
	infos := make([]AuthorizationInfo, 0, len(authorizations))
	for _, authorization := range authorizations {
		infos = append(infos, authorization.toInfo(currentID))
	}
	return infos, nil
}

func (s *Server) revokeClientAuthorization(id string, userID string, deviceID string) error {
	ctx, cancel := authContext()
	defer cancel()

	authorization, err := s.authStore.GetAuthorization(ctx, id)
	if err != nil {
		return err
	}
	if authorization.UserID != userID || authorization.DeviceID != deviceID {
		return errAuthorizationNotFound
	}

	return s.authStore.RevokeAuthorization(ctx, id, time.Now())
}

func (a ClientAuthorization) isActive(now time.Time) bool {
	if a.RevokedAt != nil {
		return false
	}
	if !a.ExpiresAt.IsZero() && !a.ExpiresAt.After(now) {
		return false
	}
	return true
}

func (a ClientAuthorization) toInfo(currentID string) AuthorizationInfo {
	return AuthorizationInfo{
		ID:         a.ID,
		UserID:     a.UserID,
		DeviceID:   a.DeviceID,
		ClientName: a.ClientName,
		RemoteAddr: a.RemoteAddr,
		CreatedAt:  a.CreatedAt,
		LastSeenAt: a.LastSeenAt,
		ExpiresAt:  a.ExpiresAt,
		RevokedAt:  a.RevokedAt,
		Current:    a.ID == currentID,
	}
}

func sortAuthorizations(authorizations []ClientAuthorization) {
	sort.Slice(authorizations, func(i, j int) bool {
		return authorizations[i].LastSeenAt.After(authorizations[j].LastSeenAt)
	})
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
