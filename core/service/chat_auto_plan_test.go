package service

import (
	"context"
	"errors"
	"testing"
	"time"

	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	plancommand "myai/core/application/chat/plan/command"
	planport "myai/core/application/chat/plan/port"
	planresult "myai/core/application/chat/plan/result"
	loadcommand "myai/core/application/session/load/command"
	messagecommand "myai/core/application/session/message/command"
	messageresult "myai/core/application/session/message/result"
	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestShouldAutoPlanRequestRecognizesDevelopmentGoals(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "Chinese feature", input: "实现一个安卓端的文件上传功能", want: true},
		{name: "Chinese terse change", input: "加上版本标识", want: true},
		{name: "Chinese UI change", input: "优化一下这个前端动画", want: true},
		{name: "English feature", input: "Implement the API endpoint for profile updates", want: true},
		{name: "English repository change", input: "Refactor the service in this repository", want: true},
		{name: "bug fix without code word", input: "修复这个 bug", want: true},
		{name: "message cleanup", input: "删除历史消息", want: true},
		{name: "question", input: "为什么这个接口返回 500？", want: false},
		{name: "implementation question", input: "为什么要重构这个项目？", want: false},
		{name: "capability question", input: "能不能修复这个安卓功能？", want: false},
		{name: "capability request", input: "能不能帮我修复这个安卓功能？", want: true},
		{name: "how question", input: "请问如何修复这个安卓功能？", want: false},
		{name: "polite imperative", input: "请修复这个 bug？", want: true},
		{name: "general writing", input: "帮我写一首诗", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAutoPlanRequest(&session.Session{Kind: session.KindUser}, tc.input); got != tc.want {
				t.Fatalf("shouldAutoPlanRequest(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestShouldAutoPlanRequestUsesRecentConversationContext(t *testing.T) {
	current := &session.Session{Kind: session.KindUser, Messages: []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"),
		domainmessage.Text(domainmessage.RoleUser, "检查这个安卓项目的连接问题"),
	}}
	if !shouldAutoPlanRequest(current, "可以，就这样修改吧") {
		t.Fatal("expected a short follow-up implementation request to use recent code context")
	}
	if shouldAutoPlanRequest(&session.Session{Kind: session.KindUser}, "修改一下") {
		t.Fatal("an isolated ambiguous edit request should not auto-plan without code context")
	}
}

func TestShouldAutoPlanRequestSkipsSubagents(t *testing.T) {
	if shouldAutoPlanRequest(&session.Session{Kind: session.KindSubagent}, "实现一个代码功能") {
		t.Fatal("subagent sessions must not recursively start autonomous planning")
	}
}

func TestRuleBasedAutoPlanClassifierReturnsStructuredIntent(t *testing.T) {
	classifier := RuleBasedAutoPlanClassifier{}

	decision, err := classifier.Classify(context.Background(), &session.Session{Kind: session.KindUser}, "实现一个安卓端功能")
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if decision.Intent != AutoPlanIntentImplementation || !decision.ShouldPlan || decision.Confidence <= 0 {
		t.Fatalf("unexpected implementation decision: %#v", decision)
	}

	decision, err = classifier.Classify(context.Background(), &session.Session{Kind: session.KindUser}, "为什么要重构这个项目？")
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if decision.Intent != AutoPlanIntentExplanation || decision.ShouldPlan {
		t.Fatalf("unexpected explanation decision: %#v", decision)
	}
}

func TestChatServiceFailsClosedWhenAutoPlanClassificationFails(t *testing.T) {
	current := &session.Session{ID: "session-1", Kind: session.KindUser, Model: "model-1", AgentMode: session.AgentModeChat}
	messages := &recordingAutoPlanMessages{current: current}
	service := NewChatService(ChatDependencies{
		AutoPlanEnabled:    true,
		AutoPlanClassifier: failingAutoPlanClassifier{},
		Models:             autoPlanModelRegistry{},
		SessionLoader:      autoPlanSessionLoader{current: current},
		MessageCommands:    messages,
		GenerationTasks:    &recordingAutoPlanGeneration{response: generationresult.GenerationResponse{Result: modelport.ChatResult{Content: "answer"}}},
	})
	if _, err := service.SendMessageStreamForSession(context.Background(), current.ID, "请修改这个项目", modelport.ChatStreamHandler{}); err != nil {
		t.Fatalf("SendMessageStreamForSession() error = %v", err)
	}
	if messages.forcePlanMode {
		t.Fatal("classifier errors must not enter autonomous planning")
	}
}

func TestModelAutoPlanClassifierParsesSemanticDecisionWithoutTools(t *testing.T) {
	model := &semanticClassifierModel{result: modelport.ChatResult{
		Content: `{"intent":"implementation","should_plan":true,"confidence":0.94,"reason":"user requests a code change"}`,
	}}
	classifier := ModelAutoPlanClassifier{
		Models:  semanticModelRegistry{model: model},
		Timeout: time.Second,
	}
	current := &session.Session{Kind: session.KindUser, Model: "model-1"}
	current.Messages = []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "检查这个项目的接口问题")}
	decision, err := classifier.Classify(context.Background(), current, "把它处理掉")
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if !decision.ShouldPlan || decision.Intent != AutoPlanIntentImplementation || decision.Confidence != 0.94 {
		t.Fatalf("unexpected semantic decision: %#v", decision)
	}
	if len(model.requests) != 1 || len(model.requests[0].Tools) != 0 {
		t.Fatalf("classifier must use one tool-free model request: %#v", model.requests)
	}
}

func TestModelAutoPlanClassifierRejectsLowConfidenceImplementation(t *testing.T) {
	model := &semanticClassifierModel{result: modelport.ChatResult{
		Content: `{"intent":"implementation","should_plan":true,"confidence":0.4,"reason":"uncertain"}`,
	}}
	classifier := ModelAutoPlanClassifier{Models: semanticModelRegistry{model: model}}
	decision, err := classifier.Classify(context.Background(), &session.Session{
		Kind: session.KindUser, Model: "model-1",
		Messages: []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "检查这个项目的接口问题")},
	}, "把它处理掉")
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if decision.ShouldPlan {
		t.Fatal("low-confidence implementation decisions must not auto-plan")
	}
}

func TestModelAutoPlanClassifierSkipsObviousTurns(t *testing.T) {
	model := &semanticClassifierModel{result: modelport.ChatResult{
		Content: `{"intent":"implementation","should_plan":true,"confidence":1,"reason":"unexpected"}`,
	}}
	classifier := ModelAutoPlanClassifier{Models: semanticModelRegistry{model: model}}
	current := &session.Session{Kind: session.KindUser, Model: "model-1"}
	decision, err := classifier.Classify(context.Background(), current, "你好，介绍一下你自己")
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if decision.ShouldPlan || len(model.requests) != 0 {
		t.Fatalf("obvious conversation should not call the classifier model: decision=%#v requests=%d", decision, len(model.requests))
	}
}

func TestModelAutoPlanClassifierUsesSemanticModelForAmbiguousContext(t *testing.T) {
	model := &semanticClassifierModel{result: modelport.ChatResult{
		Content: `{"intent":"implementation","should_plan":true,"confidence":0.88,"reason":"follow-up asks to complete the project change"}`,
	}}
	classifier := ModelAutoPlanClassifier{Models: semanticModelRegistry{model: model}}
	current := &session.Session{
		Kind: session.KindUser, Model: "model-1",
		Messages: []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "检查这个安卓项目的连接问题")},
	}
	decision, err := classifier.Classify(context.Background(), current, "把它处理掉")
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if !decision.ShouldPlan || len(model.requests) != 1 {
		t.Fatalf("ambiguous project follow-up should use semantic classification: decision=%#v requests=%d", decision, len(model.requests))
	}
}

func TestSendMessageAutomaticallyPlansAndExecutesDevelopmentRequest(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Kind: session.KindUser, Model: "model-1", AgentMode: session.AgentModeChat,
	}
	plan := &agentplan.Plan{
		ID: "plan-1", SessionID: current.ID, Goal: "implement feature", Status: agentplan.StatusDraft,
		Steps: []agentplan.Step{{ID: "step-1", Order: 1, Title: "Implement feature", Status: agentplan.StepStatusPending}},
	}
	messages := &recordingAutoPlanMessages{current: current}
	planner := &recordingAutoPlanGeneration{response: generationresult.GenerationResponse{
		SessionID: current.ID, Result: modelport.ChatResult{Content: "# Plan\n1. Implement feature"},
		Context: contextmgr.Info{WindowK: 32}, Plan: plan,
	}}
	executor := &recordingAutoPlanExecution{result: planresult.Execution{
		SessionID: current.ID, Result: modelport.ChatResult{Content: "implemented"},
		Context: contextmgr.Info{WindowK: 32}, Plan: &agentplan.Plan{ID: plan.ID, SessionID: current.ID, Status: agentplan.StatusDone, Steps: []agentplan.Step{{ID: "step-1", Order: 1, Title: "Implement feature", Status: agentplan.StepStatusDone}}},
	}}

	service := NewChatService(ChatDependencies{
		AutoPlanEnabled: true,
		Models:          autoPlanModelRegistry{},
		SessionLoader:   autoPlanSessionLoader{current: current},
		MessageCommands: messages,
		GenerationTasks: planner,
		PlanExecution:   executor,
	})
	response, err := service.SendMessageStreamForSession(context.Background(), current.ID, "实现一个代码功能", modelport.ChatStreamHandler{})
	if err != nil {
		t.Fatalf("SendMessageStreamForSession() error = %v", err)
	}
	if !messages.forcePlanMode {
		t.Fatal("expected the user turn to carry autonomous planning mode")
	}
	if len(planner.commands) != 1 || !planner.commands[0].CapturePlan || planner.commands[0].Session.AgentMode != session.AgentModePlan {
		t.Fatalf("unexpected planning generation commands: %#v", planner.commands)
	}
	if len(executor.commands) != 1 || executor.commands[0].SessionID != current.ID {
		t.Fatalf("unexpected plan execution commands: %#v", executor.commands)
	}
	if response.Result.Content != "implemented" || response.Plan == nil || response.Plan.Status != agentplan.StatusDone {
		t.Fatalf("unexpected final response: %#v", response)
	}
}

func TestSendMessagePlansWithoutExecutingWhenDecisionRequiresConfirmation(t *testing.T) {
	current := &session.Session{ID: "session-1", Kind: session.KindUser, Model: "model-1", AgentMode: session.AgentModeChat}
	plan := &agentplan.Plan{ID: "plan-1", SessionID: current.ID, Status: agentplan.StatusDraft,
		Steps: []agentplan.Step{{ID: "step-1", Order: 1, Title: "Implement", Status: agentplan.StepStatusPending}}}
	planner := &recordingAutoPlanGeneration{response: generationresult.GenerationResponse{
		SessionID: current.ID, Plan: plan, Result: modelport.ChatResult{Content: "plan ready"},
	}}
	executor := &recordingAutoPlanExecution{}
	chat := NewChatService(ChatDependencies{
		AutoPlanEnabled: true, AutoPlanClassifier: planOnlyClassifier{},
		Models: autoPlanModelRegistry{}, SessionLoader: autoPlanSessionLoader{current: current},
		MessageCommands: &recordingAutoPlanMessages{current: current},
		GenerationTasks: planner, PlanExecution: executor,
	})
	response, err := chat.SendMessageStreamForSession(context.Background(), current.ID, "实现一个功能", modelport.ChatStreamHandler{})
	if err != nil || response.Plan == nil || len(executor.commands) != 0 {
		t.Fatalf("plan-only response = %#v, executor calls = %d, error = %v", response, len(executor.commands), err)
	}
}

type planOnlyClassifier struct{}

func (planOnlyClassifier) Classify(context.Context, *session.Session, string) (AutoPlanDecision, error) {
	return AutoPlanDecision{Intent: AutoPlanIntentImplementation, ShouldPlan: true}, nil
}

func TestSendMessageResumesExistingPlanOnContinuationCommand(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Kind: session.KindUser, Model: "model-1", AgentMode: session.AgentModeChat,
		CurrentPlan: &agentplan.Plan{
			ID: "plan-1", SessionID: "session-1", Goal: "finish work", Status: agentplan.StatusFailed,
			Steps: []agentplan.Step{{ID: "step-1", Order: 1, Title: "Retry work", Status: agentplan.StepStatusFailed}},
		},
	}
	messages := &recordingAutoPlanMessages{current: current}
	executor := &recordingAutoPlanExecution{result: planresult.Execution{
		SessionID: current.ID,
		Result:    modelport.ChatResult{Content: "resumed"},
		Plan:      &agentplan.Plan{ID: "plan-1", SessionID: current.ID, Status: agentplan.StatusDone},
	}}

	service := NewChatService(ChatDependencies{
		AutoPlanEnabled: true,
		Models:          autoPlanModelRegistry{},
		SessionLoader:   autoPlanSessionLoader{current: current},
		MessageCommands: messages,
		PlanExecution:   executor,
	})
	response, err := service.SendMessageStreamForSession(context.Background(), current.ID, "继续执行", modelport.ChatStreamHandler{})
	if err != nil {
		t.Fatalf("SendMessageStreamForSession() error = %v", err)
	}
	if messages.forcePlanMode {
		t.Fatal("continuation commands must not start a new planning turn")
	}
	if len(executor.commands) != 1 || executor.commands[0].SessionID != current.ID {
		t.Fatalf("unexpected plan execution commands: %#v", executor.commands)
	}
	if response.Result.Content != "resumed" {
		t.Fatalf("unexpected resumed response: %#v", response)
	}
}

type recordingAutoPlanMessages struct {
	current       *session.Session
	forcePlanMode bool
}

func (m *recordingAutoPlanMessages) AppendUserMessage(_ context.Context, command messagecommand.AppendUserMessage) (messageresult.Command, error) {
	m.forcePlanMode = command.ForcePlanMode
	return messageresult.Command{Session: m.current, Input: command.Input, Appended: true}, nil
}

func (m *recordingAutoPlanMessages) PrepareRegeneration(context.Context, messagecommand.PrepareRegeneration) (messageresult.Command, error) {
	return messageresult.Command{}, nil
}

type recordingAutoPlanGeneration struct {
	commands []generationcommand.GenerationTask
	response generationresult.GenerationResponse
}

func (g *recordingAutoPlanGeneration) Generate(_ context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	g.commands = append(g.commands, command)
	return g.response, nil
}

type recordingAutoPlanExecution struct {
	commands []plancommand.Execute
	result   planresult.Execution
}

func (e *recordingAutoPlanExecution) Execute(_ context.Context, command plancommand.Execute, _ planport.UpdateSink) (planresult.Execution, error) {
	e.commands = append(e.commands, command)
	return e.result, nil
}

type autoPlanSessionLoader struct{ current *session.Session }

func (l autoPlanSessionLoader) Load(context.Context, string) (*session.Session, error) {
	return l.current, nil
}

func (l autoPlanSessionLoader) LoadCurrent(context.Context, string) (*session.Session, error) {
	return l.current, nil
}

func (l autoPlanSessionLoader) EnsureInMemory(context.Context, loadcommand.EnsureInMemory) (*session.Session, error) {
	return l.current, nil
}

type autoPlanModelRegistry struct{}

func (autoPlanModelRegistry) GetModel(string) modelport.ChatModelPort { return nil }
func (autoPlanModelRegistry) HasModel(string) bool                    { return false }
func (autoPlanModelRegistry) ListModels() []modelport.ModelInfo       { return nil }

type semanticModelRegistry struct{ model modelport.ChatModelPort }

func (r semanticModelRegistry) GetModel(string) modelport.ChatModelPort { return r.model }
func (r semanticModelRegistry) HasModel(string) bool                    { return r.model != nil }
func (r semanticModelRegistry) ListModels() []modelport.ModelInfo       { return nil }

type semanticClassifierModel struct {
	result   modelport.ChatResult
	requests []modelport.GenerateRequest
}

func (m *semanticClassifierModel) Generate(_ context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	m.requests = append(m.requests, request)
	return m.result, nil
}

type failingAutoPlanClassifier struct{}

func (failingAutoPlanClassifier) Classify(context.Context, *session.Session, string) (AutoPlanDecision, error) {
	return AutoPlanDecision{}, errors.New("classifier unavailable")
}
