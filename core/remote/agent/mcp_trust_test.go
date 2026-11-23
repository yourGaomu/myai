package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myai/core/mcp"
	"myai/core/remote/protocol"
	"myai/core/trust"
)

type testMCPTrustFacade struct {
	workspace string
	config    mcp.Config
}

func (f *testMCPTrustFacade) MCPTrustWorkspace() string { return f.workspace }
func (f *testMCPTrustFacade) MCPTrustCandidates(allow func(string) bool) ([]mcp.MobileTrustCandidate, error) {
	return mcp.MobileTrustCandidates(f.config, f.workspace, allow), nil
}
func (*testMCPTrustFacade) MCPTrustRuntime(_, _ string) string { return "not_loaded" }

func TestMCPTrustReviewOwnershipExpiryAndFinalAuthorization(t *testing.T) {
	workspace := t.TempDir()
	a := &Agent{}
	const endpoint = "https://test.example/mcp"
	server := mcp.ServerConfig{Name: "fixture", URL: endpoint}
	facade := &testMCPTrustFacade{workspace: workspace, config: mcp.Config{Servers: []mcp.ServerConfig{server}}}
	store := trust.NewStore(filepath.Join(t.TempDir(), "approvals.json"))
	grants := []trust.MobileMCPGrant{{Endpoint: endpoint, Generation: "current"}}
	check := func() error { return nil }
	inbound := protocol.MCPTrustInternal{PairingID: strings.Repeat("a", 64), ConnectionID: "connection", Request: protocol.MCPTrustRequest{ServerName: server.Name}}
	issue := func() string {
		t.Helper()
		request := inbound
		request.Request.Action = "review"
		var result protocol.MCPTrustResult
		if err := a.applyMCPTrust(nil, request, &result, facade, store, grants, check); err != nil || result.ReviewID == "" {
			t.Fatalf("create review: %+v %v", result, err)
		}
		return result.ReviewID
	}
	approve := func(request protocol.MCPTrustInternal, review string, check func() error) error {
		request.Request.Action, request.Request.ReviewID = "approve", review
		return a.applyMCPTrust(nil, request, &protocol.MCPTrustResult{}, facade, store, grants, check)
	}
	review := issue()
	other := inbound
	other.ConnectionID = "replacement connection"
	if err := approve(other, review, check); !errors.Is(err, errMCPTrustChanged) {
		t.Fatalf("replacement can approve: %v", err)
	}
	expired := a.mcpTrustReviews[review]
	expired.expires = time.Now().Add(-time.Second)
	a.mcpTrustReviews[review] = expired
	if err := approve(inbound, review, check); !errors.Is(err, errMCPTrustChanged) {
		t.Fatalf("expired review accepted: %v", err)
	}
	if err := approve(inbound, issue(), func() error { return errMCPTrustDenied }); !errors.Is(err, errMCPTrustDenied) {
		t.Fatalf("failed final check not enforced: %v", err)
	}
	request, err := mcp.TrustRequest(server, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if approved, err := store.Trusted(request); err != nil || approved {
		t.Fatalf("unauthorized trust was written: %v %v", approved, err)
	}
}

func TestMCPTrustCheckDoesNotFollowRedirect(t *testing.T) {
	var targetCalls int
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls++ }))
	defer target.Close()
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != protocol.MCPTrustCheckPath || r.Method != http.MethodPost {
			t.Error("unexpected check request")
		}
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer relay.Close()
	a := &Agent{config: Config{ServerURL: "ws" + strings.TrimPrefix(relay.URL, "http") + "/ws/agent", RelayToken: "test-agent-credential"}}
	if err := a.checkMCPTrustRoute(context.Background(), "test-request"); err == nil || targetCalls != 0 {
		t.Fatalf("redirect trusted or followed: %v, %d calls", err, targetCalls)
	}
	for _, invalid := range []string{"ws://relay:18080/ws/agent", "wss://secret@relay.example/ws/agent", "wss://relay.example/ws/agent?key=secret"} {
		if _, err := mcpTrustCheckURL(invalid); err == nil {
			t.Fatal("unsafe approval relay accepted")
		}
	}
}
