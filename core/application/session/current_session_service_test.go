package sessionapp

import (
	"context"
	"testing"
	"time"
)

type fakeCurrentSessionCache struct {
	userID    string
	sessionID string
	ttl       time.Duration
	deleted   bool
}

func (c *fakeCurrentSessionCache) SetCurrentSession(_ context.Context, userID string, sessionID string, ttl time.Duration) error {
	c.userID = userID
	c.sessionID = sessionID
	c.ttl = ttl
	return nil
}

func (c *fakeCurrentSessionCache) GetCurrentSession(_ context.Context, userID string) (string, error) {
	c.userID = userID
	return c.sessionID, nil
}

func (c *fakeCurrentSessionCache) DeleteCurrentSession(_ context.Context, userID string) error {
	c.userID = userID
	c.deleted = true
	return nil
}

func TestCurrentSessionServiceDelegatesToCache(t *testing.T) {
	cache := &fakeCurrentSessionCache{}
	service := CurrentSessionService{
		Cache:  cache,
		UserID: "local",
		TTL:    time.Hour,
	}

	if err := service.Save(context.Background(), "session-1"); err != nil {
		t.Fatal(err)
	}
	if cache.userID != "local" || cache.sessionID != "session-1" || cache.ttl != time.Hour {
		t.Fatalf("unexpected saved cache state: %#v", cache)
	}

	got, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "session-1" {
		t.Fatalf("unexpected session id: %s", got)
	}

	if err := service.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !cache.deleted {
		t.Fatal("expected delete to reach cache")
	}
}

func TestCurrentSessionServiceAllowsNilCache(t *testing.T) {
	service := CurrentSessionService{UserID: "local", TTL: time.Hour}
	if got, err := service.Get(context.Background()); err != nil || got != "" {
		t.Fatalf("unexpected nil cache get: %q %v", got, err)
	}
	if err := service.Save(context.Background(), "session-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
}
