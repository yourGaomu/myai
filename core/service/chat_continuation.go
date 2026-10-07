package service

import (
	"context"
	"errors"
	"strings"
)

var ErrContinuationPaused = errors.New("automatic session continuation is paused or unavailable")

type continuationState struct {
	paused bool
	cancel context.CancelFunc
}

func (s *ChatService) continuationStateLocked(id string) *continuationState {
	if s.continuations == nil {
		s.continuations = make(map[string]*continuationState)
	}
	if s.continuations[id] == nil {
		s.continuations[id] = &continuationState{}
	}
	return s.continuations[id]
}

// PauseSessionContinuations does not wait for the session operation lock:
// the running background turn may hold it while waiting for a child.
// Explicit stops survive restart; ordinary disconnects do not call this.
func (s *ChatService) PauseSessionContinuations(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	s.continuationMu.Lock()
	defer s.continuationMu.Unlock()
	state := s.continuationStateLocked(sessionID)
	state.paused = true
	if state.cancel != nil {
		state.cancel()
	}
	if s.dependencies.ContinuationControl != nil {
		return s.dependencies.ContinuationControl.SetContinuationPaused(ctx, sessionID, true)
	}
	return nil
}

// Only explicit user work re-enables continuation. Loading/restoring a session
// or receiving a child result must not silently undo a stop.
func (s *ChatService) resumeContinuations(ctx context.Context, sessionID string) error {
	s.continuationMu.Lock()
	defer s.continuationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.dependencies.ContinuationControl != nil {
		if err := s.dependencies.ContinuationControl.SetContinuationPaused(ctx, sessionID, false); err != nil {
			return err
		}
	}
	s.continuationStateLocked(sessionID).paused = false
	return nil
}

// Called only after acquiring the session lock. Registration and pause share
// a mutex, so a concurrent stop either prevents this turn or cancels it.
func (s *ChatService) beginContinuation(ctx context.Context, sessionID string) (context.Context, func(), error) {
	s.continuationMu.Lock()
	defer s.continuationMu.Unlock()
	state := s.continuationStateLocked(sessionID)
	if state.paused {
		return ctx, nil, ErrContinuationPaused
	}
	if s.dependencies.ContinuationControl != nil {
		allowed, err := s.dependencies.ContinuationControl.ContinuationAllowed(ctx, sessionID)
		if err != nil {
			return ctx, nil, err
		}
		if !allowed {
			return ctx, nil, ErrContinuationPaused
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	state.cancel = cancel
	return ctx, func() {
		cancel()
		s.continuationMu.Lock()
		state.cancel = nil
		s.continuationMu.Unlock()
	}, nil
}
