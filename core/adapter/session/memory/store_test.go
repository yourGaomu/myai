package memory

import (
	"reflect"
	"sync"
	"testing"
	"time"

	tooldef "myai/core/tool/tool"

	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	agentplan "myai/core/plan"
	domainsession "myai/core/session"
)

func TestStoreCreatesAndUpdatesCurrentSession(t *testing.T) {
	store := NewStore("gpt-default")
	if err := store.NewSession(); err != nil {
		t.Fatal(err)
	}
	current, err := store.Current()
	if err != nil {
		t.Fatal(err)
	}
	if current.ID == "" || current.Model != "gpt-default" {
		t.Fatalf("unexpected current session: %#v", current)
	}
	if err := store.AddUserMessageTo(current.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddAssistantMessageTo(current.ID, "answer"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsageTo(current.ID, llm.TokenUsage{TotalTokens: 9}); err != nil {
		t.Fatal(err)
	}
	current, err = store.Snapshot(current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Messages) != 3 || current.Usage.TotalTokens != 9 {
		t.Fatalf("unexpected updated session: %#v", current)
	}
}

func TestRegenerateSkipsTrailingEnvironmentAndPlanContext(t *testing.T) {
	store := NewStore("model")
	if err := store.NewSession(); err != nil {
		t.Fatal(err)
	}
	current, err := store.Current()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendMessagesTo(current.ID,
		domainmessage.Text(domainmessage.RoleUser, "真实问题"),
		domainmessage.EnvironmentContext("time: 12:00"),
		domainmessage.SyntheticUserText(domainmessage.SyntheticReasonPlanContext, "plan state"),
		domainmessage.Assistant("answer", "reasoning", nil),
	)
	input, err := store.TrimAfterLastUserMessage(current.ID)
	current, _ = store.Snapshot(current.ID)
	if err != nil || input != "真实问题" || len(current.Messages) != 2 {
		t.Fatalf("regenerated contextual facts instead of user input: input=%q err=%v history=%+v", input, err, current.Messages)
	}
}

func TestStoreHydratesWithoutChangingCurrentSession(t *testing.T) {
	store := NewStore("gpt-default")
	if err := store.PutSessionWithModeUsage(
		"current",
		"gpt-current",
		domainsession.AgentModeChat,
		domainsession.PermissionModeAsk,
		8,
		"",
		0,
		llm.TokenUsage{},
		llm.TokenUsage{},
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSessionWithModeUsageNoCurrent(
		"other",
		"gpt-other",
		domainsession.AgentModePlan,
		domainsession.PermissionModeReadonly,
		16,
		"summary",
		1,
		llm.TokenUsage{TotalTokens: 12},
		llm.TokenUsage{TotalTokens: 4},
		[]domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "hello")},
	); err != nil {
		t.Fatal(err)
	}
	if store.CurrentSessionId() != "current" {
		t.Fatalf("expected current session to remain unchanged, got %q", store.CurrentSessionId())
	}
	other, err := store.GetSession("other")
	if err != nil {
		t.Fatal(err)
	}
	if other.AgentMode != domainsession.AgentModePlan || other.Summary != "summary" {
		t.Fatalf("unexpected hydrated session: %#v", other)
	}
}

func TestSnapshotsAndAppendResultsAreDetachedAndSequenced(t *testing.T) {
	store := NewStore("model")
	if err := store.PutSession("session", "model", nil); err != nil {
		t.Fatal(err)
	}
	input := domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "call", Content: "original"})
	snapshot, appended, err := store.AppendMessagesTo("session", input)
	if err != nil || len(appended) != 1 || appended[0].ID == "" || appended[0].Sequence <= 0 || appended[0].CreatedAt.IsZero() {
		t.Fatalf("identity=%+v err=%v", appended, err)
	}
	snapshot.Messages[len(snapshot.Messages)-1].Parts[0].ToolResult.Content = "changed snapshot"
	appended[0].Parts[0].ToolResult.Content = "changed result"
	input.Parts[0].ToolResult.Content = "changed input"
	fresh, _ := store.Snapshot("session")
	if fresh.Messages[len(fresh.Messages)-1].Parts[0].ToolResult.Content != "original" {
		t.Fatal("store escaped through a snapshot")
	}
	const workers = 20
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := store.AppendMessagesTo("session", domainmessage.Text(domainmessage.RoleUser, "concurrent"))
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	fresh, _ = store.Snapshot("session")
	for i := 1; i < len(fresh.Messages); i++ {
		if fresh.Messages[i].Sequence <= fresh.Messages[i-1].Sequence {
			t.Fatal("nonmonotonic append")
		}
	}
}

func TestPermissionCeilingRemainsRestrictedAndPreservesEmpty(t *testing.T) {
	store := NewStore("model")
	store.PutSession("session", "model", nil)
	store.SetPermissionCeilingForSession("session", []tooldef.Permission{tooldef.PermissionRead})
	store.SetPermissionCeilingForSession("session", nil)
	fresh, _ := store.Snapshot("session")
	if len(fresh.PermissionCeiling) != 1 {
		t.Fatal("nil widened ceiling")
	}
	store.SetPermissionCeilingForSession("session", []tooldef.Permission{})
	fresh, _ = store.Snapshot("session")
	if fresh.PermissionCeiling == nil || len(fresh.PermissionCeiling) != 0 {
		t.Fatal("empty ceiling lost deny-all")
	}
}

func TestAppendIdentityChecksFullToolPayloadBeforeAnyBatchWrite(t *testing.T) {
	for _, field := range []string{"arguments", "result", "role", "reasoning", "synthetic_reason"} {
		t.Run(field, func(t *testing.T) {
			store := NewStore("model")
			if err := store.PutSession("session", "model", nil); err != nil {
				t.Fatal(err)
			}
			original := domainmessage.Assistant("answer", "reasoning", []domainmessage.ToolCall{{ID: "call", Name: "read_file", Arguments: `{"path":"a"}`}})
			if field == "result" {
				original = domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "call", Name: "read_file", Content: "original"})
			}
			original.ID = "same-id"
			_, appended, err := store.AppendMessagesTo("session", original)
			if err != nil {
				t.Fatal(err)
			}
			identical := domainmessage.Clone(appended[0])
			identical.RecordIDs = []string{"ignored"}
			identical.CreatedAt = time.Time{}
			identical.Sequence = 0
			_, retry, err := store.AppendMessagesTo("session", identical)
			if err != nil || len(retry) != 0 {
				t.Fatalf("identity metadata prevented idempotency: %v %+v", err, retry)
			}
			conflict := domainmessage.Clone(original)
			switch field {
			case "arguments":
				conflict.Parts[1].ToolCall.Arguments = `{"path":"b"}`
			case "result":
				conflict.Parts[0].ToolResult.Content = "different"
			case "role":
				conflict.Role = domainmessage.RoleUser
			case "reasoning":
				conflict.Reasoning = "different"
			case "synthetic_reason":
				conflict.SyntheticReason = domainmessage.SyntheticReasonHookContext
			}
			before, _ := store.Snapshot("session")
			if _, _, err = store.AppendMessagesTo("session", domainmessage.Text(domainmessage.RoleUser, "must not append"), conflict); err == nil {
				t.Fatal("reused identity with changed payload accepted")
			}
			after, _ := store.Snapshot("session")
			if !reflect.DeepEqual(before, after) {
				t.Fatal("conflicting batch partially wrote")
			}
		})
	}
}

func TestStoreClonesCurrentPlan(t *testing.T) {
	store := NewStore("gpt-default")
	if err := store.PutSession("session-1", "gpt-default", nil); err != nil {
		t.Fatal(err)
	}
	currentPlan := &agentplan.Plan{
		ID:    "plan-1",
		Steps: []agentplan.Step{{ID: "step-1"}},
	}
	if err := store.SetCurrentPlanForSession("session-1", currentPlan); err != nil {
		t.Fatal(err)
	}
	currentPlan.Steps[0].ID = "changed"

	stored, err := store.GetSession("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.CurrentPlan.Steps[0].ID != "step-1" {
		t.Fatal("expected memory store to clone plan input")
	}
}
