package memory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	domainauthorization "myai/core/domain/authorization"
	authorizationport "myai/core/port/authorization"
)

type Store struct {
	mu             sync.RWMutex
	authorizations map[string]domainauthorization.ClientAuthorization
	now            func() time.Time
}

func NewStore() *Store {
	return &Store{
		authorizations: make(map[string]domainauthorization.ClientAuthorization),
		now:            time.Now,
	}
}

func (s *Store) Save(_ context.Context, authorization domainauthorization.ClientAuthorization) error {
	if authorization.ID == "" {
		return errors.New("authorization id is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.authorizations[authorization.ID] = authorization
	return nil
}

func (s *Store) Get(_ context.Context, id string) (domainauthorization.ClientAuthorization, error) {
	if id == "" {
		return domainauthorization.ClientAuthorization{}, authorizationport.ErrNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	authorization, ok := s.authorizations[id]
	if !ok {
		return domainauthorization.ClientAuthorization{}, authorizationport.ErrNotFound
	}
	return authorization, nil
}

func (s *Store) Touch(_ context.Context, id string, lastSeenAt time.Time) error {
	if id == "" {
		return authorizationport.ErrNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	authorization, ok := s.authorizations[id]
	if !ok {
		return authorizationport.ErrNotFound
	}
	authorization.LastSeenAt = lastSeenAt
	s.authorizations[id] = authorization
	return nil
}

func (s *Store) Revoke(_ context.Context, id string, revokedAt time.Time) error {
	if id == "" {
		return authorizationport.ErrNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	authorization, ok := s.authorizations[id]
	if !ok {
		return authorizationport.ErrNotFound
	}
	authorization.RevokedAt = &revokedAt
	s.authorizations[id] = authorization
	return nil
}

func (s *Store) ListActive(_ context.Context, userID string, deviceID string) ([]domainauthorization.ClientAuthorization, error) {
	now := s.currentTime()

	s.mu.RLock()
	defer s.mu.RUnlock()
	authorizations := make([]domainauthorization.ClientAuthorization, 0)
	for _, authorization := range s.authorizations {
		if authorization.UserID != userID || authorization.DeviceID != deviceID {
			continue
		}
		if !authorization.ActiveAt(now) {
			continue
		}
		authorizations = append(authorizations, authorization)
	}

	sort.Slice(authorizations, func(i, j int) bool {
		return authorizations[i].LastSeenAt.After(authorizations[j].LastSeenAt)
	})
	return authorizations, nil
}

func (s *Store) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}
