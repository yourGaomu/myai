package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tooladapter "myai/core/adapter/tool/executor"
	generationcommand "myai/core/application/chat/generation/command"
	domaininteraction "myai/core/domain/interaction"
	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	"myai/core/remote/protocol"
	"myai/core/session"
	"myai/core/tool"
	tooldef "myai/core/tool/tool"
)

func TestPermissionRepliesMatchApprovalAndRequest(t *testing.T) {
	a := &Agent{permissionWaiters: newPermissionWaiterRegistry()}
	old := a.permissionWaiters.register("turn:old")
	a.permissionWaiters.unregister("turn:old", old)
	first := a.permissionWaiters.register("turn:first")
	second := a.permissionWaiters.register("turn:second")
	reply := func(request, approval string) error {
		message, err := protocol.NewMessage(protocol.TypePermissionResult, request, "user", "device", "session", protocol.PermissionResultPayload{ApprovalID: approval, Allowed: true})
		if err != nil {
			t.Fatal(err)
		}
		return a.handlePermissionResult(message)
	}
	if err := reply("turn", ""); err == nil {
		t.Fatal("accepted missing approval ID")
	}
	_ = reply("turn", "old")
	_ = reply("other-turn", "first")
	select {
	case <-first:
		t.Fatal("stale or wrong request approved first tool")
	default:
	}
	if err := reply("turn", "second"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-second:
	default:
		t.Fatal("out-of-order reply did not approve second tool")
	}
	select {
	case <-first:
		t.Fatal("second reply approved first tool")
	default:
	}
}

func TestUserQuestionRepliesAreValidatedAndFirstAnswerWins(t *testing.T) {
	registry := newUserQuestionWaiterRegistry()
	request := domaininteraction.Request{
		ID: "question-1", Questions: []domaininteraction.Question{{ID: "choice", Prompt: "Choose", Options: []string{"A", "B"}}},
	}
	waiter, err := registry.register(request)
	if err != nil {
		t.Fatal(err)
	}
	if registry.resolve(request.ID, domaininteraction.Answer{Answers: map[string]string{"choice": "C"}}) {
		t.Fatal("accepted an answer outside the offered options")
	}
	answer := domaininteraction.Answer{Answers: map[string]string{"choice": "B"}}
	if !registry.resolve(request.ID, answer) {
		t.Fatal("did not resolve a valid answer")
	}
	if registry.resolve(request.ID, domaininteraction.Answer{Answers: map[string]string{"choice": "A"}}) {
		t.Fatal("accepted a duplicate answer")
	}
	if received := <-waiter; received.Answers["choice"] != "B" {
		t.Fatalf("received the wrong answer: %#v", received)
	}
}

func TestUserQuestionListReportsResolvedKnownQuestions(t *testing.T) {
	registry := newUserQuestionWaiterRegistry()
	request := domaininteraction.Request{
		ID: "question-active", SessionID: "session-1", Questions: []domaininteraction.Question{{ID: "choice", Prompt: "Choose"}},
	}
	if _, err := registry.register(request); err != nil {
		t.Fatal(err)
	}
	active, resolved := registry.list("", []string{"question-active", "question-resolved"})
	if len(active) != 1 || active[0].ID != request.ID {
		t.Fatalf("active question was not included: %#v", active)
	}
	if len(resolved) != 1 || resolved[0] != "question-resolved" {
		t.Fatalf("stale known question was not reported: %#v", resolved)
	}
	_, resolved = registry.list("another-session", []string{request.ID})
	if len(resolved) != 0 {
		t.Fatalf("active question in another session was marked resolved: %#v", resolved)
	}
	if !registry.resolve(request.ID, domaininteraction.Answer{Answers: map[string]string{"choice": "done"}}) {
		t.Fatal("could not resolve active question")
	}
	_, resolved = registry.list("", []string{request.ID})
	if len(resolved) != 1 || resolved[0] != request.ID {
		t.Fatalf("resolved question was not reported after answer: %#v", resolved)
	}
}

func TestPermissionRuleScopeRemembersPerSessionToolAndPermissionCategory(t *testing.T) {
	a := &Agent{
		runtimes:          newSessionRuntimeManager(),
		permissionWaiters: newPermissionWaiterRegistry(),
	}
	w1 := a.permissionWaiters.registerWithTarget("req-1:app-1", permissionRuleTarget{
		sessionID: "session-a", toolName: "shell_exec", permission: "execute",
	})
	w2 := a.permissionWaiters.registerWithTarget("req-2:app-2", permissionRuleTarget{
		sessionID: "session-b", toolName: "write_file", permission: "write",
	})
	w3 := a.permissionWaiters.registerWithTarget("req-3:app-3", permissionRuleTarget{
		sessionID: "session-a", toolName: "read_file", permission: "read",
	})

	msg1, err := protocol.NewMessage(protocol.TypePermissionResult, "req-1", "user", "device", "session-a", protocol.PermissionResultPayload{
		ApprovalID: "app-1", Allowed: true, Scope: "session_permission",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.handlePermissionResult(msg1); err != nil {
		t.Fatal(err)
	}
	if !<-w1 {
		t.Fatal("expected w1 approved")
	}
	if !a.runtimes.get("session-a").isPermissionAllowed("other_exec_tool", "execute") {
		t.Fatal("expected session-a execute category to be auto-allowed")
	}
	if a.runtimes.get("session-a").isPermissionAllowed("write_file", "write") {
		t.Fatal("did not expect session-a write category to be auto-allowed")
	}
	if a.runtimes.get("session-b").isPermissionAllowed("shell_exec", "execute") {
		t.Fatal("did not expect rule to leak into session-b")
	}

	msg2, err := protocol.NewMessage(protocol.TypePermissionResult, "req-2", "user", "device", "session-b", protocol.PermissionResultPayload{
		ApprovalID: "app-2", Allowed: true, Scope: "session_tool",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.handlePermissionResult(msg2); err != nil {
		t.Fatal(err)
	}
	<-w2
	if !a.runtimes.get("session-b").isPermissionAllowed("write_file", "write") {
		t.Fatal("expected session-b write_file tool to be auto-allowed")
	}
	if a.runtimes.get("session-b").isPermissionAllowed("delete_file", "write") {
		t.Fatal("did not expect session_tool rule to allow a different write tool")
	}

	// Empty / once scope must not remember session rule
	msg3, err := protocol.NewMessage(protocol.TypePermissionResult, "req-3", "user", "device", "session-a", protocol.PermissionResultPayload{
		ApprovalID: "app-3", Allowed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.handlePermissionResult(msg3); err != nil {
		t.Fatal(err)
	}
	<-w3
	if a.runtimes.get("session-a").isPermissionAllowed("read_file", "read") {
		t.Fatal("once permission approval should not persist as session rule")
	}

	a.runtimes.get("session-a").clearPermissionRules()
	if a.runtimes.get("session-a").isPermissionAllowed("shell_exec", "execute") {
		t.Fatal("expected clearPermissionRules to reset cached session rules")
	}
}

type testConsentTool struct {
	name     string
	executed *int
}

func (t testConsentTool) Name() string                   { return t.name }
func (t testConsentTool) Description() string            { return t.name }
func (t testConsentTool) Schema() any                    { return nil }
func (t testConsentTool) Permission() tooldef.Permission { return tooldef.PermissionExecute }
func (t testConsentTool) RequiresExplicitApproval() bool { return true }
func (t testConsentTool) Call(context.Context, json.RawMessage) (tooldef.ToolOutput, error) {
	*t.executed++
	return tooldef.SuccessOutput("test tool ran"), nil
}

func replyToTestPermission(t *testing.T, a *Agent, requestID, sessionID, approvalID, scope string, allowed bool) error {
	t.Helper()
	message, err := protocol.NewMessage(protocol.TypePermissionResult, requestID, "user", "device", sessionID, protocol.PermissionResultPayload{
		ApprovalID: approvalID, Allowed: allowed, Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	return a.handlePermissionResult(message)
}

func TestForegroundPermissionEnforcesFreshConsentThroughToolExecution(t *testing.T) {
	for _, scope := range []string{"session_tool", "session_permission"} {
		t.Run(scope, func(t *testing.T) {
			const toolName, sid = "fixture_publish", "session"
			a := &Agent{runtimes: newSessionRuntimeManager(), permissionWaiters: newPermissionWaiterRegistry(), permissionTimeout: time.Second}
			runtime := a.runtimes.get(sid)
			registry := tool.NewRegisterTools()
			executed := 0
			registry.Register(testConsentTool{name: toolName, executed: &executed})
			executor := tooladapter.Executor{Registry: registry}
			seen := make(map[string]bool)
			call := func(id string, allow bool) {
				t.Helper()
				_, err := executor.Execute(context.Background(), generationcommand.ToolExecution{
					Session: &session.Session{ID: sid, PermissionMode: session.PermissionModeFull},
					Calls:   []domainmessage.ToolCall{{ID: id, Name: toolName, Arguments: `{}`}},
					Stream: llm.ChatStreamHandler{OnToolAsk: func(request llm.ToolPermissionRequest) bool {
						return a.askToolPermission(context.Background(), id, sid, request, func(payload protocol.PermissionAskPayload) error {
							if executed != 0 {
								t.Fatal("tool executed before per-call consent")
							}
							if !payload.RequireFreshConsent || payload.ApprovalID == "" || seen[payload.ApprovalID] || payload.Name != toolName {
								t.Fatalf("invalid per-call approval: %#v", payload)
							}
							seen[payload.ApprovalID] = true
							return replyToTestPermission(t, a, id, sid, payload.ApprovalID, scope, allow)
						})
					}},
				})
				if err != nil {
					t.Fatal(err)
				}
			}

			runtime.allowPermissionRule(scope, toolName, string(tooldef.PermissionExecute))
			call("deny", false)
			if len(seen) != 1 || executed != 0 {
				t.Fatal("remembered grant bypassed the foreground prompt or refusal")
			}

			runtime.clearPermissionRules()
			call("allow", true)
			if len(seen) != 2 || executed != 1 {
				t.Fatal("foreground per-call consent did not authorize exactly one execution")
			}
			if runtime.isPermissionAllowed(toolName, string(tooldef.PermissionExecute)) {
				t.Fatal("foreground per-call approval created a remembered rule")
			}
		})
	}
}

func TestForegroundPermissionRemembersAndReusesOrdinaryGrants(t *testing.T) {
	for _, scope := range []string{"session_tool", "session_permission"} {
		t.Run(scope, func(t *testing.T) {
			const sid = "session"
			a := &Agent{runtimes: newSessionRuntimeManager(), permissionWaiters: newPermissionWaiterRegistry(), permissionTimeout: time.Second}
			request := llm.ToolPermissionRequest{Name: "fixture", Permission: tooldef.PermissionWrite}
			if !a.askToolPermission(context.Background(), "grant", sid, request, func(payload protocol.PermissionAskPayload) error {
				if payload.RequireFreshConsent {
					t.Fatal("ordinary approval unexpectedly requires fresh consent")
				}
				return replyToTestPermission(t, a, "grant", sid, payload.ApprovalID, scope, true)
			}) {
				t.Fatal("ordinary approval was not accepted")
			}
			if !a.askToolPermission(context.Background(), "reuse", sid, request, func(protocol.PermissionAskPayload) error {
				t.Fatal("ordinary approval was not reused")
				return nil
			}) {
				t.Fatal("remembered grant did not allow ordinary call")
			}
		})
	}
}
