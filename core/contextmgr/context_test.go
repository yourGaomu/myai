package contextmgr

import (
	"strings"
	"testing"
	"time"

	compaction "myai/core/domain/compaction"
	domainmessage "myai/core/domain/message"
)

func TestBuildSnapshotKeepsStableSummaryPrefix(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system prompt"),
		domainmessage.Text(domainmessage.RoleUser, "old user message"),
		domainmessage.Text(domainmessage.RoleAssistant, "old assistant message"),
		domainmessage.Text(domainmessage.RoleUser, "recent user message"),
	}

	first := BuildSnapshot(messages, "stable summary", 3, 16)
	second := BuildSnapshot(messages, "stable summary", 3, 16)

	if first.Info.PrefixHash == "" {
		t.Fatal("expected prefix hash")
	}
	if first.Info.SummaryHash == "" {
		t.Fatal("expected summary hash")
	}
	if first.Info.PrefixHash != second.Info.PrefixHash {
		t.Fatalf("expected stable prefix hash, got %s and %s", first.Info.PrefixHash, second.Info.PrefixHash)
	}
	if first.Info.SummaryVersion != 2 {
		t.Fatalf("expected summary version to ignore system message, got %d", first.Info.SummaryVersion)
	}
	if first.Info.CacheableTokens != first.Info.PrefixTokens {
		t.Fatalf("expected cacheable tokens to match prefix tokens, got %d and %d", first.Info.CacheableTokens, first.Info.PrefixTokens)
	}
}

func TestShouldCompactAtThreshold(t *testing.T) {
	info := Info{
		WindowK:        1,
		SelectedTokens: 700,
	}

	if !ShouldCompact(info, DefaultCompactTriggerRatio) {
		t.Fatal("expected 70 percent threshold to trigger compaction")
	}

	info.SelectedTokens = 699
	if ShouldCompact(info, DefaultCompactTriggerRatio) {
		t.Fatal("expected compaction to stay below threshold")
	}
}

func TestBuildSnapshotCachePrefixIncludesCompletedTurnsOnly(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system prompt"),
		domainmessage.RuntimeInstruction("plan rules"),
		domainmessage.Text(domainmessage.RoleUser, "first request"),
		domainmessage.Text(domainmessage.RoleAssistant, "first answer"),
		domainmessage.RuntimeInstruction("different current rules"),
		domainmessage.Text(domainmessage.RoleUser, "second request"),
	}

	snapshot := BuildSnapshot(messages, "", 0, 16)
	wantPrefix := messages[:4]
	if snapshot.Info.PrefixHash != StableMessagesHash(wantPrefix) {
		t.Fatalf("prefix hash = %s, want completed-turn hash %s", snapshot.Info.PrefixHash, StableMessagesHash(wantPrefix))
	}
	if len(snapshot.Prefix) != len(wantPrefix) {
		t.Fatalf("prefix length = %d, want %d", len(snapshot.Prefix), len(wantPrefix))
	}

	messages[4] = domainmessage.RuntimeInstruction("changed again")
	changed := BuildSnapshot(messages, "", 0, 16)
	if changed.Info.PrefixHash != snapshot.Info.PrefixHash {
		t.Fatalf("current turn runtime changed cacheable prefix: %s != %s", changed.Info.PrefixHash, snapshot.Info.PrefixHash)
	}
}

func TestCompactSplitKeepsCompleteUserTurns(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"),
		domainmessage.RuntimeInstruction("rules"),
		domainmessage.Text(domainmessage.RoleUser, "question"),
		domainmessage.Text(domainmessage.RoleAssistant, "answer"),
		domainmessage.RuntimeInstruction("next rules"),
		domainmessage.Text(domainmessage.RoleUser, "next question"),
		domainmessage.ToolCallMessage([]domainmessage.ToolCall{{ID: "call-1", Name: "read_file"}}),
		domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "call-1", Name: "read_file", Content: "result"}),
		domainmessage.Text(domainmessage.RoleAssistant, "final answer"),
	}

	compactable, recent, cutoff := CompactSplit(messages, 1, 1)
	if cutoff != 4 || len(compactable) != 3 || !compactable[0].IsSynthetic() || compactable[1].Role != domainmessage.RoleUser || compactable[2].Role != domainmessage.RoleAssistant {
		t.Fatalf("expected the first complete turn, compactable=%#v cutoff=%d", compactable, cutoff)
	}
	if len(recent) != 5 || !recent[0].IsSynthetic() || recent[1].Role != domainmessage.RoleUser {
		t.Fatalf("expected the second complete turn to remain recent, recent=%#v", recent)
	}
}

func TestBuildSnapshotCapsLongSummary(t *testing.T) {
	longSummary := strings.Repeat("important decision ", 5000)
	snapshot := BuildSnapshot([]domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"),
		domainmessage.Text(domainmessage.RoleUser, "latest request"),
	}, longSummary, 0, 4)
	if snapshot.Info.SummaryTokens > 1024 || snapshot.Info.SelectedTokens > 4000 {
		t.Fatalf("expected bounded summary and snapshot, got info=%#v", snapshot.Info)
	}
}

func TestCurrentTurnTokensStartsAtAttachedRuntimeContext(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"),
		domainmessage.Text(domainmessage.RoleUser, "old question"),
		domainmessage.Text(domainmessage.RoleAssistant, "old answer"),
		domainmessage.RuntimeInstruction("current rules"),
		domainmessage.Text(domainmessage.RoleUser, "current question"),
		domainmessage.ToolCallMessage([]domainmessage.ToolCall{{ID: "call-1", Name: "read_file"}}),
		domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "call-1", Name: "read_file", Content: "result"}),
	}

	want := EstimateMessagesTokens(messages[3:])
	if got := CurrentTurnTokens(messages); got != want {
		t.Fatalf("current turn tokens = %d, want %d", got, want)
	}
}

func TestCompactionCheckpointRejectsChangedSourcePrefix(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"),
		domainmessage.Text(domainmessage.RoleUser, "old question"),
		domainmessage.Text(domainmessage.RoleAssistant, "old answer"),
		domainmessage.Text(domainmessage.RoleUser, "latest question"),
	}
	hash := CompactionSourceHash(messages, 3)
	if !CompactionCheckpointMatches(messages, "summary", 3, hash) {
		t.Fatal("expected matching source checkpoint")
	}
	messages[1] = domainmessage.Text(domainmessage.RoleUser, "changed question")
	if CompactionCheckpointMatches(messages, "summary", 3, hash) {
		t.Fatal("expected changed source prefix to invalidate checkpoint")
	}
}

func TestStructuredCompactionCheckpointIsAppliedOnlyForMatchingHistory(t *testing.T) {
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"),
		domainmessage.Text(domainmessage.RoleUser, "old"),
		domainmessage.Text(domainmessage.RoleAssistant, "answer"),
		domainmessage.Text(domainmessage.RoleUser, "latest"),
	}
	hash := CompactionSourceHash(messages, 3)
	checkpoint, err := compaction.NewCheckpoint(`{"current_goal":"continue","preferences":[],"constraints":[],"decisions":[],"completed_work":[],"modified_files":[],"tool_verification":[],"problems":[],"open_tasks":[],"next_steps":[],"references":[]}`, 0, 3, hash, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := BuildSnapshotWithCheckpoint(messages, checkpoint, checkpoint.SourceEndMessage, 16)
	if !snapshot.Info.HasSummary || snapshot.Info.Checkpoint == nil {
		t.Fatalf("expected checkpoint in snapshot: %#v", snapshot.Info)
	}
	if snapshot.Info.Checkpoint.SourceHistoryHash != hash {
		t.Fatalf("unexpected checkpoint hash: %#v", snapshot.Info.Checkpoint)
	}

	messages[1] = domainmessage.Text(domainmessage.RoleUser, "changed")
	stale := BuildSnapshotWithCheckpoint(messages, checkpoint, checkpoint.SourceEndMessage, 16)
	if stale.Info.HasSummary || stale.Info.Checkpoint != nil {
		t.Fatal("stale structured checkpoint was applied")
	}
}
