package relay

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	domainauthorization "myai/core/domain/authorization"
)

type AuthorizationInfo struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	DeviceID   string     `json:"device_id"`
	ClientName string     `json:"client_name"`
	RemoteAddr string     `json:"remote_addr"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	Current    bool       `json:"current"`
}

type authorizationsResponse struct {
	Authorizations []AuthorizationInfo `json:"authorizations"`
}

type revokeAuthorizationRequest struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	DeviceID    string `json:"device_id"`
	ClientToken string `json:"client_token"`
}

func authorizationInfoFromDomain(authorization domainauthorization.ClientAuthorization, currentID string) AuthorizationInfo {
	return AuthorizationInfo{
		ID:         authorization.ID,
		UserID:     authorization.UserID,
		DeviceID:   authorization.DeviceID,
		ClientName: authorization.ClientName,
		RemoteAddr: authorization.RemoteAddr,
		CreatedAt:  authorization.CreatedAt,
		LastSeenAt: authorization.LastSeenAt,
		ExpiresAt:  authorization.ExpiresAt,
		RevokedAt:  authorization.RevokedAt,
		Current:    authorization.ID == currentID,
	}
}

func (s *Server) handleAuthorizations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	clientToken := clientTokenFromRequest(r)
	if !s.validateClientToken(userID, deviceID, clientToken) {
		http.Error(w, "client token is invalid or expired", http.StatusUnauthorized)
		return
	}

	authorizations, err := s.listClientAuthorizations(userID, deviceID, clientToken)
	if err != nil {
		http.Error(w, "list authorizations failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(authorizationsResponse{Authorizations: authorizations}); err != nil {
		http.Error(w, "write authorizations response failed", http.StatusInternalServerError)
	}
}

func (s *Server) handleRevokeAuthorization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request revokeAuthorizationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid revoke request", http.StatusBadRequest)
		return
	}

	request.ID = strings.TrimSpace(request.ID)
	request.UserID = strings.TrimSpace(request.UserID)
	request.DeviceID = strings.TrimSpace(request.DeviceID)
	request.ClientToken = strings.TrimSpace(request.ClientToken)
	if request.ClientToken == "" {
		request.ClientToken = clientTokenFromRequest(r)
	}
	if request.ID == "" {
		http.Error(w, "authorization id is empty", http.StatusBadRequest)
		return
	}
	if !s.validateClientToken(request.UserID, request.DeviceID, request.ClientToken) {
		http.Error(w, "client token is invalid or expired", http.StatusUnauthorized)
		return
	}

	if err := s.revokeClientAuthorization(request.ID, request.UserID, request.DeviceID); err != nil {
		http.Error(w, "revoke authorization failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func clientTokenFromRequest(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	lowerHeader := strings.ToLower(header)
	if strings.HasPrefix(lowerHeader, "bearer ") {
		return strings.TrimSpace(header[len("bearer "):])
	}
	return strings.TrimSpace(r.URL.Query().Get("client_token"))
}
