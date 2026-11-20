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
	if memory.assistantContent != "hello" || result.AssistantMessage == nil || result.AssistantMessage.ID != "committed-answer" {
		t.Fatalf("expected the appended assistant identity and content, got %#v / %q", result.AssistantMessage, memory.assistantContent)
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
	if current.CurrentPlan != nil {
		t.Fatal("commit mutated the input snapshot")
	}
	if memory.plan == nil || memory.plan.ID != "plan-1" {
		t.Fatalf("expected plan to be persisted to memory store, got %#v", memory.plan)
	}
	if capturer.sessionID != "session-1" || capturer.goal != "write poem" || capturer.content != "- draft plan" || !capturer.now.Equal(now) {
		t.Fatalf("expected capturer inputs to be forwarded, got %#v", capturer)
	}
}

func TestResponseCommitServiceKeepsInternalPlanningOutOfChatHistory(t *testing.T) {
	memory := &responseMemoryRecorder{}
	result, err := ResponseCommitService{Memory: memory, PlanCapturer: &planCapturerRecorder{plan: &agentplan.Plan{
		ID: "plan-1", SessionID: "session-1", Status: agentplan.StatusDraft,
	}}}.Commit(CommitCommand{
		Session: responseCommitSession(session.AgentModePlan), LatestInput: "implement feature",
		Result: modelport.ChatResult{Content: "## Plan\n1. Implement"}, CapturePlan: true, Internal: true,
	})
	if err != nil || result.Plan == nil {
		t.Fatalf("expected internal plan capture, result=%#v err=%v", result, err)
	}
	if memory.assistantContent != "" || result.AssistantMessage != nil || !memory.usageWritten {
		t.Fatalf("internal planning should skip assistant history but retain usage: result=%#v memory=%#v", result, memory)
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

func (m *responseMemoryRecorder) AppendMessagesTo(_ string, messages ...domainmessage.Message) (*session.Session, []domainmessage.Message, error) {
	if m.assistantErr != nil {
		return nil, nil, m.assistantErr
	}
	message := messages[0]
	message.ID = "committed-answer"
	m.assistantContent = message.Text()
	return &session.Session{Messages: []domainmessage.Message{message}}, []domainmessage.Message{message}, nil
}

func (m *responseMemoryRecorder) Snapshot(sessionID string) (*session.Session, error) {
	return &session.Session{ID: sessionID}, nil
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
