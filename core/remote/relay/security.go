package relay

import (
	"crypto/subtle"
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
	parsed, _ := url.Parse(normalized)
	if strings.EqualFold(parsed.Host, request.Host) {
		return true
	}
	_, allowed := s.origins[normalized]
	return allowed
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
