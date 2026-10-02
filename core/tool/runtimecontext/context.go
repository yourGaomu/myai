package runtimecontext

import (
	"context"
	"errors"
	"strings"
	"sync"

	"myai/core/session"
)

type ragSettingsKey struct{}
type executionKey struct{}
type knowledgeSearchBudgetKey struct{}

// KnowledgeSearchBudget limits model-driven knowledge searches to one turn.
// It is intentionally shared through context so the tool executor can preserve
// normal ToolCall/ToolResult handling when a call is rejected.
type KnowledgeSearchBudget struct {
	mu       sync.Mutex
	maxCalls int
	calls    int
	queries  map[string]struct{}
}

func NewKnowledgeSearchBudget(maxCalls int) *KnowledgeSearchBudget {
	if maxCalls < 1 {
		maxCalls = 1
	}
	return &KnowledgeSearchBudget{maxCalls: maxCalls, queries: make(map[string]struct{})}
}

func (budget *KnowledgeSearchBudget) Reserve(query string) error {
	if budget == nil {
		return nil
	}
	key := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(query)), " "))
	if key == "" {
		return errors.New("knowledge search query is required")
	}

	budget.mu.Lock()
	defer budget.mu.Unlock()
	if _, exists := budget.queries[key]; exists {
		return errors.New("knowledge search query was already used in this turn")
	}
	if budget.calls >= budget.maxCalls {
		return errors.New("knowledge search call limit reached for this turn")
	}
	budget.calls++
	budget.queries[key] = struct{}{}
	return nil
}

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

func WithKnowledgeSearchBudget(ctx context.Context, budget *KnowledgeSearchBudget) context.Context {
	return context.WithValue(ctx, knowledgeSearchBudgetKey{}, budget)
}

func KnowledgeSearchBudgetFrom(ctx context.Context) (*KnowledgeSearchBudget, bool) {
	budget, ok := ctx.Value(knowledgeSearchBudgetKey{}).(*KnowledgeSearchBudget)
	return budget, ok && budget != nil
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
