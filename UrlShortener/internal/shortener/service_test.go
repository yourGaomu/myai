package shortener_test

import (
	"context"
	"errors"
	shortener "myai-url-shortener/internal/shortener"
	service2 "myai-url-shortener/internal/shortener/service"
	"myai-url-shortener/internal/shortener/store"
	"myai-url-shortener/internal/shortener/store/memoryStore"
	"testing"
	"time"
)

func TestServiceCreateAndResolveLink(t *testing.T) {
	service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{
		BaseURL:    "http://short.local",
		DefaultTTL: time.Hour,
	})

	response, err := service.CreateLink(context.Background(), shortener.CreateLinkRequest{
		URL: "https://example.com/file.png",
	})
	if err != nil {
		t.Fatalf("create link failed: %v", err)
	}
	if response.Code == "" {
		t.Fatalf("expected code")
	}
	if response.ShortURL != "http://short.local/s/"+response.Code {
		t.Fatalf("unexpected short url: %s", response.ShortURL)
	}

	link, err := service.Resolve(context.Background(), response.Code)
	if err != nil {
		t.Fatalf("resolve link failed: %v", err)
	}
	if link.URL != "https://example.com/file.png" {
		t.Fatalf("unexpected url: %s", link.URL)
	}
	if link.Visits != 1 {
		t.Fatalf("expected visits 1, got %d", link.Visits)
	}
}

func TestServiceRejectsInvalidURL(t *testing.T) {
	service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{})

	_, err := service.CreateLink(context.Background(), shortener.CreateLinkRequest{URL: "file:///tmp/a.png"})
	if !errors.Is(err, service2.ErrInvalidURL) {
		t.Fatalf("expected ErrInvalidURL, got %v", err)
	}
}

func TestServiceHonorsMaxVisits(t *testing.T) {
	service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{DefaultTTL: time.Hour})
	response, err := service.CreateLink(context.Background(), shortener.CreateLinkRequest{
		URL:       "https://example.com/a",
		MaxVisits: 1,
	})
	if err != nil {
		t.Fatalf("create link failed: %v", err)
	}
	if _, err := service.Resolve(context.Background(), response.Code); err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	if _, err := service.Resolve(context.Background(), response.Code); !errors.Is(err, service2.ErrVisitsExhausted) {
		t.Fatalf("expected ErrVisitsExhausted, got %v", err)
	}
}

func TestServiceDeleteLinkInfoHidesLink(t *testing.T) {
	service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{DefaultTTL: time.Hour})
	response, err := service.CreateLink(context.Background(), shortener.CreateLinkRequest{
		URL: "https://example.com/deleted",
	})
	if err != nil {
		t.Fatalf("create link failed: %v", err)
	}

	deleted, err := service.DeleteLinkInfo(context.Background(), response.Code)
	if err != nil {
		t.Fatalf("delete link failed: %v", err)
	}
	if !deleted.IsDeleted {
		t.Fatalf("expected deleted link to be marked deleted")
	}
	if _, err := service.GetLinkInfo(context.Background(), response.Code); !errors.Is(err, store.ErrLinkNotFound) {
		t.Fatalf("expected ErrLinkNotFound after delete, got %v", err)
	}
}
