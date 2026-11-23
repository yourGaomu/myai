package mcp

import (
	"testing"
)

func TestMobileTrustCandidatesUseExactSafeHTTPDeclarations(t *testing.T) {
	workspace := t.TempDir()
	endpoint := "https://mcp.example.test/api/mcp"
	t.Setenv("MYAI_TEST_REVIEW_ENDPOINT", endpoint)
	t.Setenv("MYAI_TEST_MISSING_HEADER", "")
	good := ServerConfig{Name: "candidate", URL: "${MYAI_TEST_REVIEW_ENDPOINT}", Headers: map[string]string{"Authorization": "Bearer ${MYAI_TEST_MISSING_HEADER}"}}
	config := Config{Servers: []ServerConfig{
		good,
		{Name: "stdio", Command: "not-run"},
		{Name: "mixed", URL: endpoint, Command: "not-run"},
		{Name: "disabled", URL: endpoint, Disabled: true},
		{Name: "private", URL: endpoint + "?token=secret"},
		{Name: "different", URL: "https://other.example.test/api/mcp"},
		{Name: "duplicate", URL: endpoint},
		{Name: " duplicate ", URL: endpoint, Disabled: true},
	}}
	got := MobileTrustCandidates(config, workspace, func(value string) bool { return value == endpoint })
	if len(got) != 1 || got[0].Name != "candidate" || got[0].Endpoint != endpoint || got[0].Permission != "execute" || got[0].Authentication != "headers_configured" {
		t.Fatalf("unexpected mobile candidates: %+v", got)
	}
	expected, err := TrustRequest(good, workspace)
	if err != nil || expected.Digest != got[0].Request.Digest {
		t.Fatalf("fingerprint differs from CLI: %v", err)
	}
}
