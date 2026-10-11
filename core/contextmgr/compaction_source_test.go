package contextmgr

import (
	"testing"

	domainmessage "myai/core/domain/message"
)

func TestCompactionCheckpointInvalidatesChangedAgentSource(t *testing.T) {
	message := domainmessage.SyntheticUserText(domainmessage.SyntheticReasonSubagentResult, "数据库正常")
	message.SourceID, message.SourceKind, message.SourceTaskID = "event-1", "task_result", "task-1"
	messages := []domainmessage.Message{domainmessage.Text(domainmessage.RoleSystem, "system"), message}
	hash := CompactionSourceHash(messages, len(messages))
	for _, field := range []string{"event", "kind", "task"} {
		t.Run(field, func(t *testing.T) {
			changed := domainmessage.CloneAll(messages)
			switch field {
			case "event":
				changed[1].SourceID = "event-2"
			case "kind":
				changed[1].SourceKind = "task_error"
			case "task":
				changed[1].SourceTaskID = "task-2"
			}
			if CompactionCheckpointMatches(changed, "summary", len(changed), hash) {
				t.Fatal("changed agent source reused an unrelated checkpoint")
			}
			if StableMessagesHash(changed) != StableMessagesHash(messages) {
				t.Fatal("source-only change must not alter provider text cache hash")
			}
		})
	}
	if got := CompactionSourceHash(domainmessage.CloneAll(messages), len(messages)); got != hash {
		t.Fatal("cloning changed source fingerprint")
	}
}
