package chat

import (
	"errors"
	"testing"
	"time"

	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestResponseCommitServiceWritesAssistantMessageAndUsage(t *testing.T) {
	current := responseCommitSession(session.AgentModeChat)
	memory := &responseMemoryRecorder{}

	result, err := ResponseCommitService{Memory: memory}.Commit(CommitCommand{
		Session: current,
		Result: modelport.ChatResult{
			Content: "hello",
			Usage:   modelport.TokenUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3, Available: true},
		},
		CapturePlan: true,
	})
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}

	if result.Plan != nil {
		t.Fatalf("expected no plan in chat mode, got %#v", result.Plan)
	}
	if memory.assistantContent != "hello" {
		t.Fatalf("expected assistant content to be written, got %q", memory.assistantContent)
	}
	if memory.usage.TotalTokens != 3 || !memory.usage.Available {
		t.Fatalf("expected usage to be written, got %#v", memory.usage)
	}
	if memory.plan != nil {
		t.Fatalf("expected no persisted plan, got %#v", memory.plan)
	}
}

func TestResponseCommitServiceCapturesPlanInPlanMode(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	current := responseCommitSession(session.AgentModePlan)
	memory := &responseMemoryRecorder{}
	capturer := &planCapturerRecorder{
		plan: &agentplan.Plan{
			ID:        "plan-1",
			SessionID: "session-1",
			Goal:      "write poem",
			Status:    agentplan.StatusDraft,
		},
	}

	result, err := ResponseCommitService{
		Memory:       memory,
		PlanCapturer: capturer,
		Now: func() time.Time {
			return now
		},
	}.Commit(CommitCommand{
		Session:     current,
		LatestInput: "write poem",
		Result:      modelport.ChatResult{Content: "- draft plan"},
		CapturePlan: true,
	})
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}

	if result.Plan == nil || result.Plan.ID != "plan-1" {
		t.Fatalf("expected captured plan, got %#v", result.Plan)
	}
	if current.CurrentPlan == nil || current.CurrentPlan.ID != "plan-1" {
		t.Fatalf("expected current session plan to be updated, got %#v", current.CurrentPlan)
	}
	if memory.plan == nil || memory.plan.ID != "plan-1" {
		t.Fatalf("expected plan to be persisted to memory store, got %#v", memory.plan)
	}
	if capturer.sessionID != "session-1" || capturer.goal != "write poem" || capturer.content != "- draft plan" || !capturer.now.Equal(now) {
		t.Fatalf("expected capturer inputs to be forwarded, got %#v", capturer)
	}
}

func TestResponseCommitServiceStopsWhenAssistantWriteFails(t *testing.T) {
	expected := errors.New("write failed")
	memory := &responseMemoryRecorder{assistantErr: expected}

	_, err := ResponseCommitService{Memory: memory}.Commit(CommitCommand{
		Session: responseCommitSession(session.AgentModePlan),
		Result:  modelport.ChatResult{Content: "hello"},
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected assistant write error, got %v", err)
	}
	if memory.usageWritten {
		t.Fatal("expected usage write to be skipped after assistant write failure")
	}
}

type responseMemoryRecorder struct {
	assistantContent string
	assistantErr     error
	usage            modelport.TokenUsage
	usageWritten     bool
	usageErr         error
	plan             *agentplan.Plan
	planErr          error
}

func (m *responseMemoryRecorder) AddAssistantMessageTo(sessionID string, content string) error {
	if m.assistantErr != nil {
		return m.assistantErr
	}
	m.assistantContent = content
	return nil
}

func (m *responseMemoryRecorder) AddUsageTo(sessionID string, usage modelport.TokenUsage) error {
	if m.usageErr != nil {
		return m.usageErr
	}
	m.usage = usage
	m.usageWritten = true
	return nil
}

func (m *responseMemoryRecorder) SetCurrentPlanForSession(sessionID string, currentPlan *agentplan.Plan) error {
	if m.planErr != nil {
		return m.planErr
	}
	m.plan = currentPlan
	return nil
}

type planCapturerRecorder struct {
	sessionID string
	goal      string
	content   string
	now       time.Time
	plan      *agentplan.Plan
}

func (c *planCapturerRecorder) Capture(sessionID string, goal string, content string, now time.Time) *agentplan.Plan {
	c.sessionID = sessionID
	c.goal = goal
	c.content = content
	c.now = now
	return c.plan
}

func responseCommitSession(agentMode session.AgentMode) *session.Session {
	return &session.Session{
		ID:        "session-1",
		AgentMode: agentMode,
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
		},
	}
}
