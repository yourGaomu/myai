package mcp

import (
	"strings"

	"myai/core/trust"
)

// MobileTrustCandidate is local-only. Request, including its digest, is never
// sent to Relay or Mobile; they receive a separate display projection.
type MobileTrustCandidate struct {
	Name, Endpoint, Permission, Authentication string
	Request                                    trust.Request
}

// MobileTrustCandidates uses the same normalized declaration as the CLI. It
// only resolves endpoints; it never expands header values or starts a client.
func MobileTrustCandidates(config Config, workspace string, allowEndpoint func(string) bool) []MobileTrustCandidate {
	config = NormalizeConfig(config, workspace)
	counts := make(map[string]int)
	for _, server := range config.Servers {
		counts[server.Name]++
	}
	var candidates []MobileTrustCandidate
	for _, server := range config.Servers {
		if server.Name == "" || counts[server.Name] != 1 || server.Disabled || server.Command != "" || server.URL == "" {
			continue
		}
		endpoint, err := resolveHTTPEndpoint(server.URL)
		if err != nil || !trust.SafeMobileMCPEndpoint(endpoint) || !allowEndpoint(endpoint) {
			continue
		}
		request, resolved, err := prepareTrustRequest(server, workspace)
		if err != nil || resolved != endpoint {
			continue
		}
		authentication := "none"
		if len(server.Headers) > 0 {
			authentication = "headers_configured"
		}
		candidates = append(candidates, MobileTrustCandidate{server.Name, endpoint, string(server.toolPermission()), authentication, request})
	}
	return candidates
}

// MobileTrustRuntime compares against the published runtime, not merely a
// matching service name in the last loaded config.
func (m *Manager) MobileTrustRuntime(name, digest string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if runtime := m.servers[strings.TrimSpace(name)]; runtime != nil {
		if digest != "" && runtime.client.ApprovalDigest() == digest {
			return "loaded"
		}
		return "older"
	}
	return "not_loaded"
}
