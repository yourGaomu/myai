package cacheStore

import (
	"fmt"
	"testing"
	"time"

	"myai-url-shortener/internal/shortener"
)

func TestLinkHashRoundTrip(t *testing.T) {
	expiresAt := time.Unix(200, 0).UTC()
	link := shortener.Link{
		Code:              "abc123",
		Kind:              shortener.LinkKindObject,
		URL:               "https://example.com",
		Title:             "demo",
		Scope:             "test",
		Visits:            3,
		MaxVisits:         10,
		CreatedAt:         time.Unix(100, 0).UTC(),
		UpdatedAt:         time.Unix(150, 0).UTC(),
		ExpiresAt:         &expiresAt,
		ObjectBucket:      "myai-assets",
		ObjectKey:         "uploads/a.png",
		ObjectFileName:    "a.png",
		ObjectContentType: "image/png",
		ObjectSize:        42,
	}

	values := make(map[string]string)
	for key, value := range linkHash(link) {
		values[key] = toString(value)
	}

	got := linkFromHash(values)
	if got.Code != link.Code || got.Kind != link.Kind || got.Visits != link.Visits || got.MaxVisits != link.MaxVisits {
		t.Fatalf("unexpected link after round trip: %+v", got)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected expires_at: %v", got.ExpiresAt)
	}
	if got.ObjectKey != link.ObjectKey || got.ObjectSize != link.ObjectSize {
		t.Fatalf("unexpected object fields: %+v", got)
	}
}

func toString(value interface{}) string {
	return fmt.Sprint(value)
}
