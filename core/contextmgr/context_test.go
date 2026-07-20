package contextmgr

import (
	"testing"

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
