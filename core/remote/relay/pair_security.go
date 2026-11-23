package relay

import (
	"net"
	"strings"
	"sync"
	"time"
)

const (
	bindingCodeTTL    = 5 * time.Minute
	pairRateWindow    = time.Minute
	pairIPLimit       = 10
	pairGlobalLimit   = 100
	pairIPBucketLimit = 4096
)

type bindingEntry struct {
	AgentKey  agentIdentity
	ExpiresAt time.Time
	Consumed  bool
}

type pairRateBucket struct {
	StartedAt time.Time
	Count     int
}

type pairRateLimiter struct {
	mu     sync.Mutex
	global pairRateBucket
	ips    map[string]pairRateBucket
}

// Use the TCP peer's IP; untrusted forwarding headers cannot change the bucket.
func (l *pairRateLimiter) allow(remoteAddr string, now time.Time) bool {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = strings.Trim(remoteAddr, "[]")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ips == nil {
		l.ips = make(map[string]pairRateBucket)
	}
	if now.Sub(l.global.StartedAt) >= pairRateWindow {
		l.global = pairRateBucket{StartedAt: now}
		for key, bucket := range l.ips {
			if now.Sub(bucket.StartedAt) >= pairRateWindow {
				delete(l.ips, key)
			}
		}
	}
	if l.global.Count >= pairGlobalLimit {
		return false
	}
	l.global.Count++
	bucket, exists := l.ips[ip]
	if !exists && len(l.ips) >= pairIPBucketLimit {
		return false
	}
	if now.Sub(bucket.StartedAt) >= pairRateWindow {
		bucket = pairRateBucket{StartedAt: now}
	}
	if bucket.Count >= pairIPLimit {
		return false
	}
	bucket.Count++
	l.ips[ip] = bucket
	return true
}

// Claim before issuing a durable token. Only one concurrent request can win.
// Failed token persistence also consumes the code rather than making it reusable.
func (s *Server) consumeBindingCode(code string) *agentEntry {
	s.agentLock.Lock()
	defer s.agentLock.Unlock()
	binding := s.bindings[code]
	if binding == nil || binding.Consumed || !time.Now().Before(binding.ExpiresAt) {
		return nil
	}
	agent := s.agents[binding.AgentKey]
	if agent == nil || agent.BindCode != code {
		return nil
	}
	binding.Consumed = true
	copy := *agent
	return &copy
}
