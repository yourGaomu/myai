package service

import (
	"fmt"
	"strings"

	chatcontextapi "myai/core/application/chat/context/api"
	"myai/core/contextmgr"
	agentplan "myai/core/plan"
	"myai/core/session"
)

type SnapshotService struct{}

var _ chatcontextapi.SnapshotService = SnapshotService{}

func (SnapshotService) Snapshot(current *session.Session) contextmgr.Snapshot {
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
		return contextmgr.BuildSnapshotWithCheckpoint(
			current.Messages,
			*checkpoint,
			compactedMessages,
			current.ContextWindowK,
			planSnapshot(current.CurrentPlan),
		)
	}
	return contextmgr.BuildSnapshot(
		current.Messages,
		summary,
		compactedMessages,
		current.ContextWindowK,
		planSnapshot(current.CurrentPlan),
	)
}

func planSnapshot(current *agentplan.Plan) string {
	if current == nil {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("Current execution plan snapshot:\n")
	fmt.Fprintf(&builder, "plan_id: %s\nstatus: %s\ngoal: %s\nsteps:\n", current.ID, current.Status, current.Goal)
	for _, step := range current.Steps {
		fmt.Fprintf(&builder, "- order: %d; status: %s; title: %s", step.Order, step.Status, step.Title)
		if description := strings.TrimSpace(step.Description); description != "" {
			fmt.Fprintf(&builder, "; description: %s", description)
		}
		builder.WriteByte('\n')
	}
	return contextmgr.TruncateTextToTokens(builder.String(), 512)
}
