package service

import (
	"context"

	chatcontextapi "myai/core/application/chat/context/api"
	chatcontextport "myai/core/application/chat/context/port"
	"myai/core/contextmgr"
	"myai/core/session"
)

type QueryService struct {
	Contexts chatcontextport.Provider
}

var _ chatcontextapi.QueryService = QueryService{}

func (s QueryService) Info(ctx context.Context, current *session.Session) contextmgr.Info {
	if current == nil {
		return contextmgr.Info{WindowK: contextmgr.DefaultWindowK}
	}
	if s.Contexts == nil {
		return contextmgr.Info{WindowK: contextmgr.DefaultWindowK}
	}
	return s.Contexts.Snapshot(current).Info
}
