package runtimecontext

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"

	domaininteraction "myai/core/domain/interaction"
	"myai/core/session"
)

type callIDKey struct{}
type ragSettingsKey struct{}
type executionKey struct{}
type knowledgeSearchBudgetKey struct{}
type memorySearchBudgetKey struct{}
type userQuestionHandlerKey struct{}
type workspaceInspectionKey struct{}
type askUserBudgetKey struct{}

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

// WorkspaceInspectionTracker records successful, read-only workspace queries
// during one Agent run. It lets ask_user reject claims of repository evidence
// when no corresponding inspection tool actually ran.
type WorkspaceInspectionTracker struct {
	mu      sync.RWMutex
	root    string
	checked map[string]map[string]struct{}
}

func NewWorkspaceInspectionTracker(workspaceRoot string) *WorkspaceInspectionTracker {
	root := strings.TrimSpace(workspaceRoot)
	if absolute, err := filepath.Abs(root); err == nil {
		root = filepath.Clean(absolute)
	}
	return &WorkspaceInspectionTracker{root: root, checked: make(map[string]map[string]struct{})}
}

func WithWorkspaceInspectionTracker(ctx context.Context, tracker *WorkspaceInspectionTracker) context.Context {
	if tracker == nil {
		return ctx
	}
	return context.WithValue(ctx, workspaceInspectionKey{}, tracker)
}

func WorkspaceInspectionTrackerFrom(ctx context.Context) *WorkspaceInspectionTracker {
	tracker, _ := ctx.Value(workspaceInspectionKey{}).(*WorkspaceInspectionTracker)
	return tracker
}

func (tracker *WorkspaceInspectionTracker) Record(toolName, workspaceRoot, path string) {
	if tracker == nil || (toolName != "list_files" && toolName != "read_file" && toolName != "search_files") {
		return
	}
	clean, ok := normalizeInspectionPath(tracker.root, workspaceRoot, path)
	if !ok {
		return
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.checked[clean] == nil {
		tracker.checked[clean] = make(map[string]struct{})
	}
	tracker.checked[clean][toolName] = struct{}{}
}

func (tracker *WorkspaceInspectionTracker) HasInspection(workspaceRoot, path, toolName string) bool {
	if tracker == nil {
		return false
	}
	clean, ok := normalizeInspectionPath(tracker.root, workspaceRoot, path)
	if !ok {
		return false
	}
	tracker.mu.RLock()
	defer tracker.mu.RUnlock()
	_, ok = tracker.checked[clean][toolName]
	return ok
}

func normalizeInspectionPath(trackerRoot, workspaceRoot, value string) (string, bool) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		root = trackerRoot
	}
	if root == "" {
		return "", false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	rootAbs = filepath.Clean(rootAbs)
	if trackerRoot != "" {
		trackerAbs, err := filepath.Abs(trackerRoot)
		if err != nil {
			return "", false
		}
		if relative, err := filepath.Rel(rootAbs, trackerAbs); err != nil || relative != "." {
			return "", false
		}
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = "."
	}
	joined := value
	if !filepath.IsAbs(joined) {
		joined = filepath.Join(rootAbs, joined)
	}
	joined, err = filepath.Abs(joined)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(rootAbs, filepath.Clean(joined))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(filepath.Clean(relative)), true
}

// AskUserBudget permits a second round only after the first was answered.
type AskUserBudget struct {
	mu          sync.Mutex
	reasons     map[string]struct{}
	inProgress  bool
	canFollowUp bool
}

func NewAskUserBudget() *AskUserBudget { return &AskUserBudget{reasons: make(map[string]struct{})} }

func WithAskUserBudget(ctx context.Context, budget *AskUserBudget) context.Context {
	if budget == nil {
		return ctx
	}
	return context.WithValue(ctx, askUserBudgetKey{}, budget)
}

func AskUserBudgetFrom(ctx context.Context) *AskUserBudget {
	budget, _ := ctx.Value(askUserBudgetKey{}).(*AskUserBudget)
	return budget
}

func (budget *AskUserBudget) Reserve(reason string) error {
	if budget == nil {
		return nil
	}
	reason = strings.ToLower(strings.Join(strings.Fields(reason), " "))
	if reason == "" {
		return errors.New("ask_user decision reason is required")
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if budget.inProgress {
		return errors.New("ask_user is already waiting for an answer")
	}
	if len(budget.reasons) >= 2 {
		return errors.New("ask_user round limit reached for this Agent run")
	}
	if len(budget.reasons) == 1 && !budget.canFollowUp {
		return errors.New("ask_user cannot ask again after a skipped or unanswered round")
	}
	if _, exists := budget.reasons[reason]; exists {
		return errors.New("ask_user cannot repeat the same decision in one Agent run")
	}
	budget.reasons[reason] = struct{}{}
	budget.inProgress = true
	return nil
}

func (budget *AskUserBudget) Finish(answered bool) {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	budget.inProgress = false
	budget.canFollowUp = answered
}

type Execution struct {
	ToolCallID      string
	SessionID       string
	ParentSessionID string
	TaskID          string
	RequestID       string
	RunID           string
	ParentRunID     string
	PlanID          string
	StepID          string
	WorkspaceRoot   string
	SandboxID       string
	ModelID         string
	IsSubagent      bool
}

func WithUserQuestionHandler(ctx context.Context, handler domaininteraction.UserQuestionHandler) context.Context {
	if handler == nil {
		return ctx
	}
	return context.WithValue(ctx, userQuestionHandlerKey{}, handler)
}

func UserQuestionHandlerFrom(ctx context.Context) domaininteraction.UserQuestionHandler {
	handler, _ := ctx.Value(userQuestionHandlerKey{}).(domaininteraction.UserQuestionHandler)
	return handler
}

// WithCallID 为每个工具调用保留稳定身份，嵌套 spawn_agent 可以关联父调用。
func WithCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, callIDKey{}, id)
}

func CallID(ctx context.Context) string {
	id, _ := ctx.Value(callIDKey{}).(string)
	return id
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

// MemorySearchBudget limits model-driven AI memory searches to one generation turn.
type MemorySearchBudget struct {
	mu       sync.Mutex
	maxCalls int
	calls    int
	queries  map[string]struct{}
}

func NewMemorySearchBudget(maxCalls int) *MemorySearchBudget {
	if maxCalls < 1 {
		maxCalls = 1
	}
	return &MemorySearchBudget{maxCalls: maxCalls, queries: make(map[string]struct{})}
}

func (budget *MemorySearchBudget) Reserve(query string) error {
	if budget == nil {
		return nil
	}
	key := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(query)), " "))
	if key == "" {
		return errors.New("memory search query is required")
	}

	budget.mu.Lock()
	defer budget.mu.Unlock()
	if _, exists := budget.queries[key]; exists {
		return errors.New("memory search query was already used in this turn")
	}
	if budget.calls >= budget.maxCalls {
		return errors.New("memory search call limit reached for this turn")
	}
	budget.calls++
	budget.queries[key] = struct{}{}
	return nil
}

func WithMemorySearchBudget(ctx context.Context, budget *MemorySearchBudget) context.Context {
	return context.WithValue(ctx, memorySearchBudgetKey{}, budget)
}

func MemorySearchBudgetFrom(ctx context.Context) (*MemorySearchBudget, bool) {
	budget, ok := ctx.Value(memorySearchBudgetKey{}).(*MemorySearchBudget)
	return budget, ok && budget != nil
}

func WithExecution(ctx context.Context, execution Execution) context.Context {
	return context.WithValue(ctx, executionKey{}, execution)
}

func CurrentExecution(ctx context.Context) (Execution, bool) {
	execution, ok := ctx.Value(executionKey{}).(Execution)
	// 1. 批次上下文保留默认 ID；执行器派生的当前调用 ID 优先覆盖它。
	if callID := CallID(ctx); callID != "" {
		execution.ToolCallID = callID
	}
	return execution, ok
}

func WorkspaceRoot(ctx context.Context, fallback string) string {
	if execution, ok := CurrentExecution(ctx); ok && execution.WorkspaceRoot != "" {
		return execution.WorkspaceRoot
	}
	return fallback
}
