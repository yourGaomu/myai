package service

import (
	"errors"
	"strings"
)

var ErrUserAcceptanceCapacity = errors.New("pending user persistence capacity is full")

func (s *ChatService) reserveUserAcceptance(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	s.userAcceptanceMu.Lock()
	defer s.userAcceptanceMu.Unlock()
	if s.userAcceptanceSlots == nil {
		s.userAcceptanceSlots = make(map[string]bool)
	}
	if _, exists := s.userAcceptanceSlots[sessionID]; exists {
		return nil
	}
	if len(s.userAcceptanceSlots) >= maxUserAcceptanceSlots {
		return ErrUserAcceptanceCapacity
	}
	s.userAcceptanceSlots[sessionID] = false
	return nil
}

// Keep an active turn's reservation between accepted batches. A failed batch
// retains its slot after the turn ends, until retry or explicit clear succeeds.
func (s *ChatService) finishUserAcceptance(sessionID string) {
	s.userAcceptanceMu.Lock()
	defer s.userAcceptanceMu.Unlock()
	if _, exists := s.userAcceptanceSlots[sessionID]; !exists {
		return
	}
	s.userAcceptanceSlots[sessionID] = false
	if _, pending := s.pendingUserAcceptance[sessionID]; !pending {
		delete(s.userAcceptanceSlots, sessionID)
	}
}

func (s *ChatService) releaseUserAcceptance(sessionID string) {
	s.userAcceptanceMu.Lock()
	if !s.userAcceptanceSlots[sessionID] {
		delete(s.userAcceptanceSlots, sessionID)
	}
	s.userAcceptanceMu.Unlock()
}
