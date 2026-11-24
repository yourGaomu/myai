package contextmgr

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	compaction "myai/core/domain/compaction"
	domainmessage "myai/core/domain/message"
)

func TestRequestSnapshotProjectsCurrentContextAndPreservesSourceHistory(t *testing.T) {
	legacy := domainmessage.SyntheticText(domainmessage.SyntheticReasonRuntimeInstruction, "old plan rules")
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "fixed policy"), legacy,
		domainmessage.RAGContext("obsolete evidence"), domainmessage.Text(domainmessage.RoleUser, "old question"),
		domainmessage.EnvironmentContext("old environment"), domainmessage.Text(domainmessage.RoleAssistant, "old answer"),
		domainmessage.RuntimeInstruction("stale current rules"), domainmessage.Text(domainmessage.RoleUser, "current question"),
	}
	original := domainmessage.CloneAll(messages)
	hash := CompactionSourceHash(messages, 6)
	checkpoint, err := compaction.NewCheckpoint(`{"current_goal":"continue","preferences":[],"constraints":[],"decisions":[],"completed_work":[],"modified_files":[],"tool_verification":[],"problems":[],"open_tasks":[],"next_steps":[],"references":[]}`, 0, 6, hash, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	options := RequestOptions{Budget: NewRequestBudget(16000, 1200, 2048), TurnContext: TurnContext{RuntimeInstruction: "current chat rules", EnvironmentContext: "current environment"}}
	snapshot := BuildSnapshotForRequest(messages, checkpoint, 6, options)
	if snapshot.Info.DroppedMessages != 0 || snapshot.Info.Checkpoint == nil || snapshot.Info.Checkpoint.SourceHistoryHash != hash {
		t.Fatalf("projection changed checkpoint meaning: %#v", snapshot.Info)
	}
	var foundCurrentRules, foundCurrentEnvironment bool
	for index, message := range snapshot.Messages {
		if message.Role == domainmessage.RoleSystem && index != 0 {
			t.Fatal("runtime state was promoted to system")
		}
		foundCurrentRules = foundCurrentRules || strings.Contains(message.Text(), "current chat rules")
		foundCurrentEnvironment = foundCurrentEnvironment || strings.Contains(message.Text(), "current environment")
		if strings.Contains(message.Text(), "stale current rules") || strings.Contains(message.Text(), "old environment") {
			t.Fatalf("superseded current-turn context survived: %s", message.Text())
		}
	}
	if !foundCurrentRules || !foundCurrentEnvironment {
		t.Fatalf("current context or completed history missing: %#v", snapshot.Messages)
	}
	if !reflect.DeepEqual(messages, original) || CompactionSourceHash(messages, 6) != hash {
		t.Fatal("request projection mutated durable history")
	}
	snapshot.Messages[len(snapshot.Messages)-1].Parts[0].Text = "changed request copy"
	if !reflect.DeepEqual(messages, original) {
		t.Fatal("snapshot aliases durable history")
	}
	messages[2].Parts[0].Text = "changed raw source"
	if CompactionCheckpointMatchesCheckpoint(messages, &checkpoint) {
		t.Fatal("filtered source changes must still invalidate a checkpoint")
	}
}

func TestRequestProjectionPreservesSkillAccountingOnExistingRuntimeMessage(t *testing.T) {
	const runtimeText = "active runtime instruction"
	history := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "fixed rules"),
		domainmessage.RuntimeInstruction("old instruction"),
		domainmessage.Text(domainmessage.RoleUser, "old input"),
		domainmessage.Text(domainmessage.RoleAssistant, "old answer"),
		domainmessage.RuntimeInstruction(runtimeText),
		domainmessage.Text(domainmessage.RoleUser, "current input"),
	}
	history[1].SkillPromptTokens = 7
	history[4].SkillPromptTokens = 3
	options := RequestOptions{
		Budget: NewRequestBudget(16000, 0, 0),
		TurnContext: TurnContext{
			RuntimeInstruction: runtimeText,
			SkillPromptTokens:  history[4].SkillPromptTokens,
		},
	}
	projected := BuildSnapshotForRequest(history, compaction.Checkpoint{}, 0, options)
	options.TurnContext.SkillPromptTokens = 0
	unattributed := BuildSnapshotForRequest(history, compaction.Checkpoint{}, 0, options)
	if len(projected.Messages) != 6 || !projected.Messages[4].IsSyntheticReason(domainmessage.SyntheticReasonRuntimeInstruction) || projected.Messages[4].SkillPromptTokens != 3 {
		t.Fatalf("expected the active runtime message to carry its Skill estimate: %#v", projected.Messages)
	}
	projected.Messages[4].SkillPromptTokens = 0
	if !reflect.DeepEqual(projected.Messages, unattributed.Messages) || projected.Info.SelectedTokens != unattributed.Info.SelectedTokens {
		t.Fatal("Skill accounting changed wire content or its token estimate")
	}
	if got := (TurnContext{RuntimeInstruction: domainmessage.RuntimeTurnBoundaryPrompt, SkillPromptTokens: 3}).Messages(); len(got) != 0 {
		t.Fatalf("expected a legacy boundary-only prompt to remain omitted: %#v", got)
	}
}

func TestRequestBudgetUsesExactModelWindowAndAllReservations(t *testing.T) {
	for _, test := range []struct{ session, model, want int }{
		{0, 200000, 200000}, {128, 32768, 32768}, {16, 200000, 16000}, {0, 0, 16000}, {128, 0, 128000},
	} {
		if got := ResolveWindowTokens(test.session, test.model); got != test.want {
			t.Fatalf("window(%d,%d)=%d want %d", test.session, test.model, got, test.want)
		}
	}
	budget := NewRequestBudget(8192, 700, 1500)
	messages := []domainmessage.Message{domainmessage.Text(domainmessage.RoleSystem, "policy"), domainmessage.Text(domainmessage.RoleUser, "question")}
	if budget.ReservedTokens() != 2264 || budget.AvailableTokens(messages) != 8192-2264-EstimateMessagesTokens(messages) {
		t.Fatal("shared budget excluded tool schemas, output or protocol margin")
	}
	if EstimateTextTokens("    \n    ") == 0 || EstimateTextTokens("x\n    y") <= EstimateTextTokens("xy") {
		t.Fatal("whitespace was excluded from token accounting")
	}
	if NewRequestBudget(4000, 1500, 3000).Validate(messages) == nil {
		t.Fatal("output reservation exceeded model capacity without error")
	}
}

func TestCompactSplitInsideOneLongUserTurnPreservesParallelToolPairs(t *testing.T) {
	messages := []domainmessage.Message{domainmessage.Text(domainmessage.RoleSystem, "policy"), domainmessage.Text(domainmessage.RoleUser, "finish the original task")}
	for index := 0; index < 12; index++ {
		calls := []domainmessage.ToolCall{{ID: fmt.Sprintf("%d-a", index), Name: "read_file"}, {ID: fmt.Sprintf("%d-b", index), Name: "check_file"}}
		messages = append(messages, domainmessage.Assistant("inspect", "reasoning", calls))
		for _, call := range calls {
			messages = append(messages, domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: strings.Repeat("文", 700)}))
		}
	}
	options := RequestOptions{Budget: NewRequestBudget(16000, 1200, 2048), TurnContext: TurnContext{RuntimeInstruction: "current rules"}}
	compactable, recent, cutoff, err := CompactSplitForRequest(messages, 0, options, 8)
	if err != nil || len(compactable) == 0 || cutoff <= 2 || len(recent) == 0 {
		t.Fatalf("long single turn did not compact: cutoff=%d err=%v", cutoff, err)
	}
	if err := ValidateToolPairs(compactable); err != nil {
		t.Fatalf("source split a parallel exchange: %v", err)
	}
	if err := ValidateToolPairs(recent); err != nil {
		t.Fatalf("retained suffix split a parallel exchange: %v", err)
	}
	summary := compaction.LegacyCheckpoint("completed earlier tool work", cutoff, CompactionSourceHash(messages, cutoff))
	snapshot := BuildSnapshotForRequest(messages, summary, cutoff, options)
	if err := options.Budget.ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("compressed request failed shared budget: %v", err)
	}
	var preserved bool
	for _, message := range snapshot.Messages {
		preserved = preserved || message.Text() == "finish the original task"
	}
	if !preserved || snapshot.Info.DroppedMessages != 0 {
		t.Fatal("active task or unsummarized history was lost")
	}
}

func TestRequestBudgetRejectsSilentHistoryDropAndBrokenPairs(t *testing.T) {
	messages := []domainmessage.Message{domainmessage.Text(domainmessage.RoleSystem, "policy")}
	for index := 0; index < 6; index++ {
		messages = append(messages, domainmessage.Text(domainmessage.RoleUser, strings.Repeat("文", 1000)), domainmessage.Text(domainmessage.RoleAssistant, "answer"))
	}
	options := RequestOptions{Budget: NewRequestBudget(4000, 400, 500)}
	snapshot := BuildSnapshotForRequest(messages, compaction.Checkpoint{}, 0, options)
	if snapshot.Info.DroppedMessages == 0 || options.Budget.ValidateSnapshot(snapshot) == nil {
		t.Fatal("history was silently omitted without compaction")
	}
	for _, malformed := range [][]domainmessage.Message{
		{domainmessage.ToolCallMessage([]domainmessage.ToolCall{{ID: "missing"}})},
		{domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "orphan"})},
	} {
		if ValidateToolPairs(malformed) == nil {
			t.Fatal("malformed tool exchange passed validation")
		}
	}
}

func TestToolResultLimitKeepsSmallBytesAndReportsExhaustion(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "large", Content: strings.Repeat("大", 6000)}),
		domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "small", Content: "  ok\n"}),
		domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "error", Status: "error", ErrorCode: "failed", ErrorMessage: "actual error"}),
	}
	limited, err := LimitToolResults(messages, 1000)
	if err != nil || EstimateMessagesTokens(limited) > 1000 {
		t.Fatalf("bounded results failed: %v", err)
	}
	small, _ := limited[1].FirstToolResult()
	if small.Content != "  ok\n" || small.PromptTruncated {
		t.Fatal("small untruncated result changed its whitespace")
	}
	exhausted, err := LimitToolResults(messages, 0)
	if err == nil {
		t.Fatal("exhausted budget did not return an error")
	}
	large, _ := exhausted[0].FirstToolResult()
	failure, _ := exhausted[2].FirstToolResult()
	if large.Content == "" || failure.ErrorMessage != "actual error" || large.FullContent != strings.Repeat("大", 6000) {
		t.Fatal("exhaustion cleared useful text or audit content")
	}
}
