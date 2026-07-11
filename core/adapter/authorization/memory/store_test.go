package memory

import (
	"context"
	"testing"
	"time"

	domainauthorization "myai/core/domain/authorization"
)

func TestStoreListsOnlyActiveAuthorizations(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	store := NewStore()
	store.now = func() time.Time { return now }
	revokedAt := now.Add(-time.Minute)
	values := []domainauthorization.ClientAuthorization{
		{ID: "active", UserID: "user-1", DeviceID: "device-1", LastSeenAt: now, ExpiresAt: now.Add(time.Hour)},
		{ID: "expired", UserID: "user-1", DeviceID: "device-1", LastSeenAt: now, ExpiresAt: now.Add(-time.Hour)},
		{ID: "revoked", UserID: "user-1", DeviceID: "device-1", LastSeenAt: now, ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt},
		{ID: "other", UserID: "user-2", DeviceID: "device-1", LastSeenAt: now, ExpiresAt: now.Add(time.Hour)},
	}
	for _, authorization := range values {
		if err := store.Save(context.Background(), authorization); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	authorizations, err := store.ListActive(context.Background(), "user-1", "device-1")
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(authorizations) != 1 || authorizations[0].ID != "active" {
		t.Fatalf("unexpected active authorizations: %#v", authorizations)
	}
}
