package local

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domaininteraction "myai/core/domain/interaction"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

func askUserTestArgs() json.RawMessage {
	return json.RawMessage(`{"decision_impact":"high","decision_reason":"The decision changes the amount of work and cannot be inferred.","context_evidence":[{"source":"conversation","finding":"The user did not specify scope."}],"title":"Scope","questions":[{"id":"scope","prompt":"How much?","multi_select":false,"allow_other":true,"choices":[{"label":"small"},{"label":"large","description":"Enough detail","recommended":true}]}]}`)
}

func TestAskUserToolWaitsForStructuredAnswer(t *testing.T) {
	var received domaininteraction.Request
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SessionID: "session-1", ToolCallID: "call-1", RunID: "run-1", TaskID: "task-1"})
	ctx = toolruntime.WithUserQuestionHandler(ctx, func(_ context.Context, request domaininteraction.Request) (domaininteraction.Answer, error) {
		received = request
		return domaininteraction.Answer{Structured: map[string]domaininteraction.QuestionAnswer{"scope": {Selected: []string{"large"}}}}, nil
	})
	output, err := NewAskUserTool().Call(ctx, askUserTestArgs())
	if err != nil {
		t.Fatal(err)
	}
	if received.SessionID != "session-1" || received.SourceTaskID != "task-1" || received.ToolCallID != "call-1" || received.RunID != "run-1" {
		t.Fatalf("question lost execution identity: %#v", received)
	}
	if output.Status != "success" || output.Content != `{"structured_answers":{"scope":{"selected":["large"]}}}` {
		t.Fatalf("unexpected tool output: %#v", output)
	}
}

func TestAskUserRejectsMissingContextEvidenceAndLegacyPayloads(t *testing.T) {
	tool := NewAskUserTool()
	valid := string(askUserTestArgs())
	for name, args := range map[string]string{
		"missing evidence":    strings.Replace(valid, `,"context_evidence":[{"source":"conversation","finding":"The user did not specify scope."}]`, "", 1),
		"legacy option field": strings.Replace(valid, `"choices":[{"label":"small"},{"label":"large","description":"Enough detail","recommended":true}]`, `"options":["small","large"]`, 1),
		"unknown field":       strings.Replace(valid, `"title":"Scope"`, `"title":"Scope","answers":{"scope":"small"}`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tool.Call(context.Background(), json.RawMessage(args)); err == nil {
				t.Fatal("invalid call was accepted")
			}
		})
	}
}

func TestAskUserRequiresWorkspaceEvidenceToHaveBeenInspected(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("Project overview"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SessionID: "s", WorkspaceRoot: root})
	ctx = toolruntime.WithWorkspaceInspectionTracker(ctx, toolruntime.NewWorkspaceInspectionTracker(root))
	ctx = toolruntime.WithUserQuestionHandler(ctx, func(_ context.Context, _ domaininteraction.Request) (domaininteraction.Answer, error) {
		return domaininteraction.Answer{Cancelled: true}, nil
	})
	ask := func(path, toolName string, wantSuccess bool) {
		t.Helper()
		args := json.RawMessage(`{"decision_impact":"high","decision_reason":"Need a brand choice.","context_evidence":[{"source":"workspace","path":"` + path + `","tool":"` + toolName + `","finding":"No branding found."}],"title":"Choose","questions":[{"id":"style","prompt":"Style?","multi_select":false,"choices":[{"label":"A"}]}]}`)
		output, err := NewAskUserTool().Call(ctx, args)
		if wantSuccess {
			if err != nil || output.Content != `{"skipped":true}` {
				t.Fatalf("verified evidence for %s %s was rejected: output=%#v error=%v", toolName, path, output, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), "not successfully inspected") {
			t.Fatalf("unverified evidence for %s %s was not rejected: %v", toolName, path, err)
		}
	}
	ask("README.md", "read_file", false)
	if _, err := NewListFilesToolWithWorkspace(root).Call(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	ask(".", "list_files", true)
	ask("README.md", "list_files", false)
	ask("README.md", "read_file", false)
	read := NewReadFileToolWithWorkspace(root)
	if _, err := read.Call(ctx, json.RawMessage(`{"path":"missing.md"}`)); err == nil {
		t.Fatal("a missing file was read successfully")
	}
	ask("missing.md", "read_file", false)
	if _, err := read.Call(ctx, json.RawMessage(`{"path":"README.md"}`)); err != nil {
		t.Fatal(err)
	}
	ask("README.md", "read_file", true)
	ask("README.md", "search_files", false)
	search := NewSearchFilesToolWithWorkspace(root)
	if _, err := search.Call(ctx, json.RawMessage(`{"query":"branding","path":"missing.md"}`)); err == nil {
		t.Fatal("a missing file was searched successfully")
	}
	ask("missing.md", "search_files", false)
	if _, err := search.Call(ctx, json.RawMessage(`{"query":"branding","path":"README.md"}`)); err != nil {
		t.Fatal(err)
	}
	ask("README.md", "search_files", true)
}

func TestAskUserQuestionLimitAndCancelOutput(t *testing.T) {
	ctx := toolruntime.WithAskUserBudget(context.Background(), toolruntime.NewAskUserBudget())
	ctx = toolruntime.WithUserQuestionHandler(ctx, func(_ context.Context, _ domaininteraction.Request) (domaininteraction.Answer, error) {
		return domaininteraction.Answer{Cancelled: true}, nil
	})
	args := json.RawMessage(`{"decision_impact":"high","decision_reason":"A consequential choice is unresolved.","context_evidence":[{"source":"conversation","finding":"User has not specified this."}],"safe_default_if_skipped":"Use the existing project convention.","title":"Choose","questions":[{"id":"q","prompt":"Pick?","multi_select":false,"choices":[{"label":"A"}]}]}`)
	output, err := NewAskUserTool().Call(ctx, args)
	if err != nil || output.Content != `{"skipped":true,"safe_default_if_skipped":"Use the existing project convention."}` {
		t.Fatalf("output=%#v error=%v", output, err)
	}
	if _, err := NewAskUserTool().Call(ctx, json.RawMessage(strings.Replace(string(args), "A consequential choice is unresolved.", "Another consequential choice is unresolved.", 1))); err == nil || !strings.Contains(err.Error(), "skipped or unanswered") {
		t.Fatalf("second call error = %v", err)
	}
}

func TestAskUserToolCapabilities(t *testing.T) {
	if NewAskUserTool().Permission() != tooldef.PermissionRead || !NewAskUserTool().IsTurnBarrier() || !NewAskUserTool().RequiresUserInteraction() {
		t.Fatal("ask_user must be a read-only interactive turn barrier")
	}
}

func TestAskUserRejectsSubagentCalls(t *testing.T) {
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SessionID: "child", IsSubagent: true})
	ctx = toolruntime.WithUserQuestionHandler(ctx, func(_ context.Context, _ domaininteraction.Request) (domaininteraction.Answer, error) {
		return domaininteraction.Answer{}, nil
	})
	args := json.RawMessage(`{"decision_impact":"high","decision_reason":"A consequential choice is unresolved.","context_evidence":[{"source":"conversation","finding":"No preference supplied."}],"title":"Choose","questions":[{"id":"q","prompt":"Pick?","multi_select":false,"choices":[{"label":"A"}]}]}`)
	if _, err := NewAskUserTool().Call(ctx, args); err == nil || !strings.Contains(err.Error(), "subagents") {
		t.Fatalf("subagent ask error = %v", err)
	}
}

func TestAskUserAllowsFollowUpOnlyAfterAnswer(t *testing.T) {
	tool := NewAskUserTool()
	first := askUserTestArgs()
	second := json.RawMessage(strings.Replace(string(first), "The decision changes the amount of work and cannot be inferred.", "The service integration depends on the answer.", 1))
	third := json.RawMessage(strings.Replace(string(second), "The service integration depends on the answer.", "Another distinct decision.", 1))
	ctx := toolruntime.WithAskUserBudget(context.Background(), toolruntime.NewAskUserBudget())
	calls := 0
	ctx = toolruntime.WithUserQuestionHandler(ctx, func(ctx context.Context, _ domaininteraction.Request) (domaininteraction.Answer, error) {
		calls++
		if calls == 1 {
			if _, err := tool.Call(ctx, second); err == nil || !strings.Contains(err.Error(), "already waiting") {
				t.Fatalf("overlapping ask error = %v", err)
			}
		}
		return domaininteraction.Answer{Structured: map[string]domaininteraction.QuestionAnswer{"scope": {Selected: []string{"small"}}}}, nil
	})
	if _, err := tool.Call(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(ctx, first); err == nil || !strings.Contains(err.Error(), "repeat") {
		t.Fatalf("repeated ask error = %v", err)
	}
	if _, err := tool.Call(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(ctx, third); err == nil || !strings.Contains(err.Error(), "round limit") {
		t.Fatalf("third ask error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("handler received %d calls", calls)
	}
}

func TestAskUserRejectsFollowUpAfterFailedAnswer(t *testing.T) {
	failure := errors.New("question delivery failed")
	ctx := toolruntime.WithAskUserBudget(context.Background(), toolruntime.NewAskUserBudget())
	ctx = toolruntime.WithUserQuestionHandler(ctx, func(_ context.Context, _ domaininteraction.Request) (domaininteraction.Answer, error) {
		return domaininteraction.Answer{}, failure
	})
	if _, err := NewAskUserTool().Call(ctx, askUserTestArgs()); !errors.Is(err, failure) {
		t.Fatalf("ask error = %v", err)
	}
	if _, err := NewAskUserTool().Call(ctx, askUserTestArgs()); err == nil || !strings.Contains(err.Error(), "skipped or unanswered") {
		t.Fatalf("follow-up ask error = %v", err)
	}
}

func TestAskUserRespectsExplicitlyDisabledCustomAnswer(t *testing.T) {
	ctx := toolruntime.WithUserQuestionHandler(context.Background(), func(_ context.Context, request domaininteraction.Request) (domaininteraction.Answer, error) {
		if request.Questions[0].AllowOther {
			t.Fatal("explicitly disabled custom answer was enabled")
		}
		return domaininteraction.Answer{Cancelled: true}, nil
	})
	args := strings.Replace(string(askUserTestArgs()), `"allow_other":true`, `"allow_other":false`, 1)
	if _, err := NewAskUserTool().Call(ctx, json.RawMessage(args)); err != nil {
		t.Fatal(err)
	}
	args = strings.Replace(args, `[{"label":"small"},{"label":"large","description":"Enough detail","recommended":true}]`, `[]`, 1)
	if _, err := NewAskUserTool().Call(ctx, json.RawMessage(args)); err == nil || !strings.Contains(err.Error(), "requires a choice or a custom answer") {
		t.Fatalf("unanswerable question error = %v", err)
	}
}
