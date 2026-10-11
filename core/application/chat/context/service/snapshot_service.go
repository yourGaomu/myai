package service

import (
	chatcontextapi "myai/core/application/chat/context/api"
	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	"myai/core/session"
)

type SnapshotService struct{}

var _ chatcontextapi.SnapshotService = SnapshotService{}

func (SnapshotService) Snapshot(current *session.Session) contextmgr.Snapshot {
	return (SnapshotService{}).SnapshotWithBudget(current, 0)
}

func (SnapshotService) SnapshotWithBudget(current *session.Session, reservedTokens int) contextmgr.Snapshot {
	if current == nil {
		return contextmgr.Snapshot{}
	}
	summary := current.Summary
	compactedMessages := current.CompactedMessages
	checkpoint := current.CompactionCheckpoint
	if checkpoint != nil {
		if !contextmgr.CompactionCheckpointMatchesCheckpoint(current.Messages, checkpoint) {
			checkpoint = nil
			summary = ""
			compactedMessages = 0
		}
	} else if !contextmgr.CompactionCheckpointMatches(current.Messages, summary, compactedMessages, current.CompactionSourceHash) {
		// A stale checkpoint is worse than a larger prompt: never apply a summary
		// to a message prefix that no longer matches its source history.
		summary = ""
		compactedMessages = 0
	}
	if checkpoint != nil {
		summary = checkpoint.Summary
		compactedMessages = checkpoint.SourceEndMessage
	}
	if checkpoint != nil {
		return contextmgr.BuildSnapshotWithBudget(
			current.Messages,
			*checkpoint,
			compactedMessages,
			current.ContextWindowK,
			reservedTokens,
		)
	}
	return contextmgr.BuildSnapshotWithBudget(
		current.Messages,
		compaction.LegacyCheckpoint(summary, compactedMessages, ""),
		compactedMessages,
		current.ContextWindowK,
		reservedTokens,
	)
}
