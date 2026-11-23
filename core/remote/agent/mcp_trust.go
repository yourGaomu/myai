package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"myai/core/mcp"
	"myai/core/remote/protocol"
	"myai/core/trust"
)

const (
	mcpTrustReviewTTL  = 2 * time.Minute
	maxMCPTrustReviews = 256
)

var (
	errMCPTrustDenied  = errors.New("mcp trust denied")
	errMCPTrustChanged = errors.New("mcp trust review required")
)

type mcpTrustFacade interface {
	MCPTrustWorkspace() string
	MCPTrustCandidates(func(string) bool) ([]mcp.MobileTrustCandidate, error)
	MCPTrustRuntime(name, digest string) string
}

type mcpTrustReview struct {
	pairing, connection, generation, server, digest string
	agent                                           *websocket.Conn
	expires                                         time.Time
}

func (a *Agent) handleMCPTrust(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	var inbound protocol.MCPTrustInternal
	if len(message.Payload) > protocol.MCPTrustMaxRequestBytes || json.Unmarshal(message.Payload, &inbound) != nil ||
		inbound.Request.Validate() != nil || !trust.ValidMobilePairingID(inbound.PairingID) ||
		len(inbound.ConnectionID) == 0 || len(inbound.ConnectionID) > 128 || message.ClientToken != "" ||
		message.UserID != a.config.UserID || message.DeviceID != a.config.DeviceID {
		return errMCPTrustDenied
	}
	result := protocol.MCPTrustResult{Action: inbound.Request.Action, ServerName: inbound.Request.ServerName}
	// Also serializes with catalog reload. Release the lock before writing to
	// the socket, which is shared with permission and chat replies.
	a.catalogMu.Lock()
	err := a.runMCPTrust(ctx, conn, message.RequestID, inbound, &result)
	a.catalogMu.Unlock()
	switch {
	case errors.Is(err, errMCPTrustDenied):
		result.Code = protocol.MCPTrustDenied
	case errors.Is(err, errMCPTrustChanged):
		result.Code = protocol.MCPTrustReviewRequired
	case err != nil:
		result.Code = protocol.MCPTrustUnavailable
	}
	if result.Code != "" {
		result.Services, result.ReviewID = nil, ""
	}
	return a.writeRemoteMessage(conn, protocol.TypeMCPTrustResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) runMCPTrust(ctx context.Context, conn *websocket.Conn, requestID string, inbound protocol.MCPTrustInternal, result *protocol.MCPTrustResult) error {
	facade, ok := a.mcpManager.(mcpTrustFacade)
	if !ok || facade.MCPTrustWorkspace() == "" {
		return errMCPTrustDenied
	}
	admins, err := trust.DefaultMobileMCPStore()
	if err != nil {
		return err
	}
	approvals, err := trust.DefaultStore()
	if err != nil {
		return err
	}
	scope := trust.MobileMCPScope{Workspace: facade.MCPTrustWorkspace(), UserID: a.config.UserID, DeviceID: a.config.DeviceID}
	return admins.WithGrants(scope, inbound.PairingID, func(grants []trust.MobileMCPGrant) error {
		if len(grants) == 0 {
			return errMCPTrustDenied
		}
		check := func() error { return a.checkMCPTrustRoute(ctx, requestID) }
		return a.applyMCPTrust(conn, inbound, result, facade, approvals, grants, check)
	})
}

func (a *Agent) applyMCPTrust(conn *websocket.Conn, inbound protocol.MCPTrustInternal, result *protocol.MCPTrustResult, facade mcpTrustFacade, approvals *trust.Store, grants []trust.MobileMCPGrant, check func() error) error {
	req := inbound.Request
	allowed := make(map[string]string, len(grants))
	for _, grant := range grants {
		allowed[grant.Endpoint] = grant.Generation
	}
	load := func() ([]mcp.MobileTrustCandidate, error) {
		return facade.MCPTrustCandidates(func(endpoint string) bool { return allowed[endpoint] != "" })
	}
	project := func(c mcp.MobileTrustCandidate, approved bool) protocol.MCPTrustService {
		return protocol.MCPTrustService{
			Name: c.Name, Endpoint: c.Endpoint, Permission: c.Permission, Authentication: c.Authentication, Trusted: approved,
			Runtime: facade.MCPTrustRuntime(c.Name, c.Request.Digest),
		}
	}
	for id, review := range a.mcpTrustReviews {
		if !time.Now().Before(review.expires) || review.agent != conn {
			delete(a.mcpTrustReviews, id)
		}
	}
	if req.Action == "approve" {
		review, ok := a.mcpTrustReviews[req.ReviewID]
		if !ok || review.pairing != inbound.PairingID || review.connection != inbound.ConnectionID || review.server != req.ServerName {
			return errMCPTrustChanged
		}
		delete(a.mcpTrustReviews, req.ReviewID)
		items, err := load()
		if err != nil {
			return err
		}
		for _, c := range items {
			if c.Name != review.server || c.Request.Digest != review.digest || allowed[c.Endpoint] != review.generation {
				continue
			}
			if err := approvals.ApproveIf(c.Request, func() error {
				// Re-read under the trust write lock. CLI edits and a queued or
				// delayed confirmation must never approve a replacement.
				latest, err := load()
				if err != nil {
					return err
				}
				for _, entry := range latest {
					if entry.Name != c.Name || entry.Request.Digest != c.Request.Digest || !time.Now().Before(review.expires) {
						continue
					}
					if err := check(); err != nil {
						return err
					}
					if !time.Now().Before(review.expires) {
						return errMCPTrustChanged
					}
					return nil
				}
				return errMCPTrustChanged
			}); err != nil {
				return err
			}
			result.Services = []protocol.MCPTrustService{project(c, true)}
			return nil
		}
		return errMCPTrustChanged
	}
	if err := check(); err != nil {
		return err
	}
	items, err := load()
	if err != nil {
		return err
	}
	approved, err := approvals.List(facade.MCPTrustWorkspace(), trust.KindMCP)
	if err != nil {
		return err
	}
	digests := make(map[string]bool, len(approved))
	for _, approval := range approved {
		digests[approval.Digest] = true
	}
	for _, c := range items {
		if !protocol.ValidMCPTrustServerName(c.Name) || req.ServerName != "" && c.Name != req.ServerName {
			continue
		}
		if len(result.Services) == protocol.MCPTrustMaxServices {
			return errors.New("too many mobile MCP declarations")
		}
		result.Services = append(result.Services, project(c, digests[c.Request.Digest]))
		if req.Action == "review" {
			if len(a.mcpTrustReviews) >= maxMCPTrustReviews {
				return errors.New("mcp trust review limit reached")
			}
			id := uuid.NewString()
			if a.mcpTrustReviews == nil {
				a.mcpTrustReviews = make(map[string]mcpTrustReview)
			}
			a.mcpTrustReviews[id] = mcpTrustReview{inbound.PairingID, inbound.ConnectionID, allowed[c.Endpoint], c.Name, c.Request.Digest, conn, time.Now().Add(mcpTrustReviewTTL)}
			result.ReviewID = id
			return nil
		}
	}
	if req.Action == "review" {
		return errMCPTrustChanged
	}
	return nil
}

var mcpTrustHTTPClient = &http.Client{
	Timeout: 5 * time.Second,
	// Use the Relay directly; this request carries the Agent credential.
	Transport:     &http.Transport{ForceAttemptHTTP2: true},
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}

func mcpTrustCheckURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Hostname() == "" || u.Opaque != "" || u.Path != "/ws/agent" || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery {
		return "", errMCPTrustDenied
	}
	switch u.Scheme {
	case "wss":
		u.Scheme = "https"
	case "ws":
		ip := net.ParseIP(u.Hostname())
		if !strings.EqualFold(u.Hostname(), "localhost") && (ip == nil || !ip.IsLoopback()) {
			return "", errMCPTrustDenied
		}
		u.Scheme = "http"
	default:
		return "", errMCPTrustDenied
	}
	u.Path, u.RawPath = protocol.MCPTrustCheckPath, ""
	return u.String(), nil
}

func (a *Agent) checkMCPTrustRoute(ctx context.Context, requestID string) error {
	endpoint, err := mcpTrustCheckURL(a.config.ServerURL)
	if err != nil {
		return err
	}
	body, err := json.Marshal(protocol.MCPTrustCheck{RequestID: requestID})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.config.RelayToken))
	req.Header.Set(protocol.HeaderAgentUserID, a.config.UserID)
	req.Header.Set(protocol.HeaderAgentDeviceID, a.config.DeviceID)
	response, err := mcpTrustHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNoContent:
		return ctx.Err()
	case http.StatusUnauthorized, http.StatusForbidden:
		return errMCPTrustDenied
	default:
		return errors.New("could not verify mcp trust route")
	}
}
