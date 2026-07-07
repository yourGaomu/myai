package shortener_test

import (
	"bytes"
	"encoding/json"
	shortener "myai-url-shortener/internal/shortener"
	"myai-url-shortener/internal/shortener/handle"
	service2 "myai-url-shortener/internal/shortener/service"
	"myai-url-shortener/internal/shortener/store/memoryStore"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHandlerCreateAndRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{
		BaseURL:    "http://short.local",
		DefaultTTL: time.Hour,
	})
	router := handle.NewRouter(service)

	body, err := json.Marshal(shortener.CreateLinkRequest{URL: "https://example.com/a.png"})
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}
	createRequest := httptest.NewRequest(http.MethodPost, "/api/links", bytes.NewReader(body))
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}

	var payload shortener.CreateLinkResponse
	if err := json.NewDecoder(createResponse.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	redirectRequest := httptest.NewRequest(http.MethodGet, "/s/"+payload.Code, nil)
	redirectResponse := httptest.NewRecorder()
	router.ServeHTTP(redirectResponse, redirectRequest)
	if redirectResponse.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, redirectResponse.Code)
	}
	if got := redirectResponse.Header().Get("Location"); got != "https://example.com/a.png" {
		t.Fatalf("unexpected location: %s", got)
	}
}
