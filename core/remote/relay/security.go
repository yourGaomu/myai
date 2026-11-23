package relay

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"

	"myai/core/remote/protocol"
)

type agentIdentity struct {
	UserID   string
	DeviceID string
}

func (s *Server) authenticateAgentRequest(request *http.Request) (agentIdentity, bool) {
	if s == nil || request == nil {
		return agentIdentity{}, false
	}
	userID := strings.TrimSpace(request.Header.Get(protocol.HeaderAgentUserID))
	deviceID := strings.TrimSpace(request.Header.Get(protocol.HeaderAgentDeviceID))
	expected, found := s.agentCredentials[agentKey(userID, deviceID)]
	if !found || expected == "" {
		return agentIdentity{}, false
	}
	const prefix = "Bearer "
	header := strings.TrimSpace(request.Header.Get("Authorization"))
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return agentIdentity{}, false
	}
	provided := strings.TrimSpace(header[len(prefix):])
	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return agentIdentity{}, false
	}
	return agentIdentity{UserID: userID, DeviceID: deviceID}, true
}

func (s *Server) originAllowed(request *http.Request) bool {
	if request == nil {
		return false
	}
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	normalized, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	if s.allowAllOrigins {
		return true
	}
	parsed, _ := url.Parse(normalized)
	if strings.EqualFold(parsed.Host, request.Host) {
		return true
	}
	if _, allowed := s.origins[normalized]; allowed {
		return true
	}
	if loopbackKey, ok := loopbackOriginKey(parsed); ok {
		_, allowed := s.loopbackOrigins[loopbackKey]
		return allowed
	}
	return false
}

func normalizeOrigin(origin string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}

// Loopback wildcard patterns are intentionally limited to localhost addresses.
// They make Expo's changing development port usable without allowing arbitrary
// websites to call a public Relay.
func normalizeLoopbackOriginPattern(origin string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(origin))
	scheme, hostPattern, found := strings.Cut(value, "://")
	if !found || (scheme != "http" && scheme != "https") || !strings.HasSuffix(hostPattern, ":*") {
		return "", false
	}
	host := strings.TrimSuffix(hostPattern, ":*")
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if !isLoopbackHost(host) {
		return "", false
	}
	return scheme + "://" + host, true
}

func loopbackOriginKey(origin *url.URL) (string, bool) {
	if origin == nil {
		return "", false
	}
	host := strings.ToLower(origin.Hostname())
	if !isLoopbackHost(host) {
		return "", false
	}
	return strings.ToLower(origin.Scheme) + "://" + host, true
}

func loopbackListenAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	return err == nil && isLoopbackHost(host)
}

// MCP trust accepts real TLS or explicitly enabled, direct loopback development.
// In particular, X-Forwarded-Proto is not proof that the peer used TLS.
func (s *Server) mcpTrustTransportAllowed(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !s.mcpTrustLocalDev {
		return false
	}
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(remote); err != nil || ip == nil || !ip.IsLoopback() {
		return false
	}
	localAddr, ok := r.Context().Value(http.LocalAddrContextKey).(*net.TCPAddr)
	if !ok || !localAddr.IP.IsLoopback() {
		return false
	}
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = strings.TrimPrefix(strings.TrimSuffix(r.Host, "]"), "[")
	}
	if !isLoopbackHost(host) {
		return false
	}
	for key := range r.Header {
		key = strings.ToLower(key)
		if key == "forwarded" || key == "via" || key == "x-real-ip" || strings.HasPrefix(key, "x-forwarded-") {
			return false
		}
	}
	return true
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
