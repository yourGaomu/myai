package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReconnectBackoffGrowsWithBoundedJitter(t *testing.T) {
	backoff := time.Second
	for _, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second} {
		if backoff != want {
			t.Fatalf("backoff %s want %s", backoff, want)
		}
		for i := 0; i < 20; i++ {
			delay := reconnectDelay(backoff)
			if delay < backoff/2 || delay > backoff {
				t.Fatalf("jitter %s outside [%s,%s]", delay, backoff/2, backoff)
			}
		}
		backoff = nextReconnectBackoff(backoff)
	}
}

func TestRelayDialPermanentFailuresAndTransientFailures(t *testing.T) {
	for _, tc := range []struct {
		status    int
		permanent bool
	}{
		{400, true}, {401, true}, {403, true}, {404, true}, {408, false}, {429, false}, {500, false}, {503, false},
	} {
		err := relayDialError(errors.New("handshake failed"), &http.Response{StatusCode: tc.status, Status: http.StatusText(tc.status)})
		if permanentRelayError(err) != tc.permanent {
			t.Fatalf("status %d permanent=%v", tc.status, permanentRelayError(err))
		}
	}
	if permanentRelayError(errors.New("temporary network outage")) {
		t.Fatal("network outage was treated as permanent")
	}
}

func TestAgentUnauthorizedConnectionDoesNotRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unauthorized", http.StatusUnauthorized) }))
	defer server.Close()
	a := &Agent{config: Config{ServerURL: "ws" + strings.TrimPrefix(server.URL, "http"), RelayToken: "fake-test-token", UserID: "user", DeviceID: "device"}}
	connected, err := a.runConnection(context.Background())
	if connected || !permanentRelayError(err) {
		t.Fatalf("unauthorized dial: connected=%v err=%v", connected, err)
	}
}
