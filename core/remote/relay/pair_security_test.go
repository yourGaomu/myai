package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBindingCodeAtomicConsumeAndReconnect(t *testing.T) {
	s := newTestServer()
	original := &peer{}
	s.registerAgent(original, "local", "pc-local", "123456", "test")
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.consumeBindingCode("123456") != nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("pairing winners = %d, want 1", wins.Load())
	}
	s.unregisterAgent(original, "local", "pc-local")
	s.registerAgent(&peer{}, "local", "pc-local", "123456", "reconnect")
	if s.consumeBindingCode("123456") != nil {
		t.Fatal("reconnect revived consumed code")
	}
}

func TestBindingCodeExpiresWithoutReconnectExtensionAndKeepsFirstOwner(t *testing.T) {
	s := newTestServer()
	original := &peer{}
	s.registerAgent(original, "local", "pc-local", "123456", "test")
	expires := s.bindings["123456"].ExpiresAt
	s.registerAgent(&peer{}, "other-user", "other-device", "123456", "collision")
	if s.bindings["123456"].AgentKey != agentKey("local", "pc-local") {
		t.Fatal("collision stole code")
	}
	if got := s.consumeBindingCode("123456"); got == nil || got.UserID != "local" {
		t.Fatal("code lost original owner")
	}
	s.agentLock.Lock()
	s.bindings["123456"].Consumed = false
	s.bindings["123456"].ExpiresAt = time.Now().Add(-time.Second)
	s.agentLock.Unlock()
	s.registerAgent(&peer{}, "local", "pc-local", "123456", "reconnect")
	if s.consumeBindingCode("123456") != nil {
		t.Fatal("expired code was refreshed")
	}
	if expires.After(time.Now().Add(bindingCodeTTL)) {
		t.Fatal("code TTL is longer than expected")
	}
}

func TestPairRateLimitsIPAndGlobalAndRecovers(t *testing.T) {
	now := time.Now()
	var byIP pairRateLimiter
	for i := 0; i < pairIPLimit; i++ {
		if !byIP.allow(fmt.Sprintf("192.0.2.1:%d", 1000+i), now) {
			t.Fatal("IP rejected too soon")
		}
	}
	if byIP.allow("192.0.2.1:9000", now) {
		t.Fatal("source port bypassed IP limit")
	}
	if !byIP.allow("192.0.2.2:9000", now) {
		t.Fatal("IP limit blocked unrelated source")
	}
	if !byIP.allow("192.0.2.1:9000", now.Add(pairRateWindow)) {
		t.Fatal("IP limit did not recover")
	}
	var global pairRateLimiter
	for i := 0; i < pairGlobalLimit; i++ {
		if !global.allow(fmt.Sprintf("192.0.2.%d:1000", i), now) {
			t.Fatal("global rejected too soon")
		}
	}
	if global.allow("198.51.100.1:1000", now) {
		t.Fatal("new IP bypassed global limit")
	}
}

func TestPairHTTPLimitsFailuresAndIgnoresForwardedIP(t *testing.T) {
	s := newTestServer()
	for i := 0; i <= pairIPLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"bind_code":"000000"}`))
		req.RemoteAddr = fmt.Sprintf("192.0.2.1:%d", 1000+i)
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		response := httptest.NewRecorder()
		s.handlePair(response, req)
		want := http.StatusNotFound
		if i == pairIPLimit {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("attempt %d status %d want %d", i, response.Code, want)
		}
	}
}
