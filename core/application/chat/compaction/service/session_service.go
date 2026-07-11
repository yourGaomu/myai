package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	compactionapi "myai/core/application/chat/compaction/api"
	compactioncommand "myai/core/application/chat/compaction/command"
	compactionport "myai/core/application/chat/compaction/port"
	chatport "myai/core/application/chat/port"
	"myai/core/contextmgr"
)

type SessionService struct {
	Sessions  compactionport.SessionLoader
	Models    chatport.ModelProvider
	Compactor compactionport.SessionCompactor
	Contexts  compactionport.ContextQuery
}

var _ compactionapi.SessionService = SessionService{}

func (s SessionService) Compact(ctx context.Context, command compactioncommand.CompactSession) (contextmgr.Info, error) {
	if s.Sessions == nil {
		return contextmgr.Info{}, errors.New("session manager is nil")
	}
	if s.Models == nil {
		return contextmgr.Info{}, errors.New("llm client is nil")
	}
	if s.Compactor == nil {
		return contextmgr.Info{}, errors.New("session compactor is nil")
	}
	if s.Contexts == nil {
		return contextmgr.Info{}, errors.New("context query is nil")
	}
	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return contextmgr.Info{}, errors.New("session id is empty")
	}
	current, err := s.Sessions.Load(ctx, sessionID)
	if err != nil {
		return contextmgr.Info{}, err
	}
	model := s.Models.GetModel(current.Model)
	if model == nil {
		return contextmgr.Info{}, fmt.Errorf("model not found: %s", current.Model)
	}
	if err := s.Compactor.CompactSession(ctx, current, model); err != nil {
		if errors.Is(err, ErrNotEnoughHistory) {
			return s.Contexts.Info(ctx, current), err
		}
		return contextmgr.Info{}, err
	}
	return s.Contexts.Info(ctx, current), nil
}
