package protocol

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	TypeMCPTrust         MessageType = "mcp_trust"
	TypeMCPTrustInternal MessageType = "mcp_trust_internal"
	TypeMCPTrustResult   MessageType = "mcp_trust_result"
	MCPTrustCheckPath                = "/mcp/trust/check"

	MCPTrustMaxRequestBytes  = 2048
	MCPTrustMaxResponseBytes = 64 * 1024
	MCPTrustMaxServices      = 32

	MCPTrustDenied         = "mcp_trust_denied"
	MCPTrustUnavailable    = "mcp_trust_unavailable"
	MCPTrustReviewRequired = "mcp_trust_review_required"
)

type MCPTrustRequest struct {
	Action     string `json:"action"`
	ServerName string `json:"server_name,omitempty"`
	ReviewID   string `json:"review_id,omitempty"`
}

func (r MCPTrustRequest) Validate() error {
	validName := validMCPTrustText(r.ServerName, 128)
	validReview := validMCPTrustText(r.ReviewID, 64)
	switch {
	case r.Action == "list" && r.ServerName == "" && r.ReviewID == "":
	case r.Action == "review" && validName && r.ReviewID == "":
	case r.Action == "approve" && validName && validReview:
	default:
		return errors.New("invalid mcp trust request")
	}
	return nil
}

func validMCPTrustText(value string, size int) bool {
	return len(value) > 0 && len(value) <= size && strings.TrimSpace(value) == value && utf8.ValidString(value) &&
		!strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) })
}

func ValidMCPTrustServerName(name string) bool { return validMCPTrustText(name, 128) }

type MCPTrustInternal struct {
	Request      MCPTrustRequest `json:"request"`
	PairingID    string          `json:"pairing_id"`
	ConnectionID string          `json:"connection_id"`
}

type MCPTrustCheck struct {
	RequestID string `json:"request_id"`
}

type MCPTrustResult struct {
	Action     string            `json:"action"`
	Code       string            `json:"code,omitempty"`
	ServerName string            `json:"server_name,omitempty"`
	ReviewID   string            `json:"review_id,omitempty"`
	Services   []MCPTrustService `json:"services,omitempty"`
}

type MCPTrustService struct {
	Name           string `json:"name"`
	Endpoint       string `json:"endpoint"`
	Permission     string `json:"permission"`
	Authentication string `json:"authentication"`
	Trusted        bool   `json:"trusted"`
	Runtime        string `json:"runtime"`
}
