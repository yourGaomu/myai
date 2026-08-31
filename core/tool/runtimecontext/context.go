package runtimecontext

import (
	"context"

	"myai/core/session"
)

type ragSettingsKey struct{}
type executionKey struct{}

type Execution struct {
	SessionID     string
	TaskID        string
	RequestID     string
	RunID         string
	ParentRunID   string
	PlanID        string
	StepID        string
	WorkspaceRoot string
	SandboxID     string
	ModelID       string
}

func WithRAGSettings(ctx context.Context, settings session.RAGSettings) context.Context {
	return context.WithValue(ctx, ragSettingsKey{}, session.CloneRAGSettings(settings))
}

func RAGSettings(ctx context.Context) (session.RAGSettings, bool) {
	settings, ok := ctx.Value(ragSettingsKey{}).(session.RAGSettings)
	if !ok {
		return session.RAGSettings{}, false
	}
	return session.CloneRAGSettings(settings), true
}

func WithExecution(ctx context.Context, execution Execution) context.Context {
	return context.WithValue(ctx, executionKey{}, execution)
}

func CurrentExecution(ctx context.Context) (Execution, bool) {
	execution, ok := ctx.Value(executionKey{}).(Execution)
	return execution, ok
}

func WorkspaceRoot(ctx context.Context, fallback string) string {
	if execution, ok := CurrentExecution(ctx); ok && execution.WorkspaceRoot != "" {
		return execution.WorkspaceRoot
	}
	return fallback
}
