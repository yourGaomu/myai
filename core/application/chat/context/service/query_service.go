package service

import (
	"context"

	chatcontextapi "myai/core/application/chat/context/api"
	chatcontextport "myai/core/application/chat/context/port"
	"myai/core/contextmgr"
	"myai/core/session"
)

type QueryService struct {
	Contexts            chatcontextport.Provider
	RuntimeInstructions chatcontextport.RuntimeInstructionProvider
}

var _ chatcontextapi.QueryService = QueryService{}

func (s QueryService) Info(ctx context.Context, current *session.Session) contextmgr.Info {
	if current == nil {
		return contextmgr.Info{WindowK: contextmgr.DefaultWindowK}
	}
	return s.InfoWithRuntimePrompt(current, s.runtimePrompt(ctx, current))
}

func (s QueryService) InfoWithRuntimePrompt(current *session.Session, runtimePrompt string) contextmgr.Info {
	if current == nil || s.Contexts == nil {
		return contextmgr.Info{WindowK: contextmgr.DefaultWindowK}
	}
	return s.Contexts.Snapshot(current, runtimePrompt).Info
}

func (s QueryService) runtimePrompt(ctx context.Context, current *session.Session) string {
	if s.RuntimeInstructions == nil {
		return ""
	}
	return s.RuntimeInstructions.Prompt(ctx, current, "", false)
}
