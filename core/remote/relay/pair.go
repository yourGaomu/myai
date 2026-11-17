package relay

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type pairRequest struct {
	BindCode   string `json:"bind_code"`
	ClientName string `json:"client_name"`
}

type pairResponse struct {
	UserID      string `json:"user_id"`
	DeviceID    string `json:"device_id"`
	ClientToken string `json:"client_token"`
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.pairLimiter.allow(r.RemoteAddr, time.Now()) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "too many pair attempts; retry later", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var request pairRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid pair request", http.StatusBadRequest)
		return
	}

	bindCode := strings.TrimSpace(request.BindCode)
	if bindCode == "" {
		http.Error(w, "bind code is empty", http.StatusBadRequest)
		return
	}

	agent := s.consumeBindingCode(bindCode)
	if agent == nil {
		http.Error(w, "bind code is invalid or expired", http.StatusNotFound)
		return
	}

	clientToken, _, err := s.authorizeClient(agent.UserID, agent.DeviceID, request.ClientName, r.RemoteAddr)
	if err != nil {
		http.Error(w, "authorize client failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(pairResponse{
		UserID:      agent.UserID,
		DeviceID:    agent.DeviceID,
		ClientToken: clientToken,
	}); err != nil {
		http.Error(w, "write pair response failed", http.StatusInternalServerError)
	}
}
