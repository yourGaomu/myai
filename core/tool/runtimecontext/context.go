package runtimecontext

import (
	"context"

	"myai/core/session"
)

type key struct{}

func WithRAGSettings(ctx context.Context, settings session.RAGSettings) context.Context {
	return context.WithValue(ctx, key{}, session.CloneRAGSettings(settings))
}

func RAGSettings(ctx context.Context) (session.RAGSettings, bool) {
	settings, ok := ctx.Value(key{}).(session.RAGSettings)
	if !ok {
		return session.RAGSettings{}, false
	}
	return session.CloneRAGSettings(settings), true
}
