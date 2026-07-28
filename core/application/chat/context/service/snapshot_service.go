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
	return contextmgr.BuildSnapshot(
		current.Messages,
		current.Summary,
		current.CompactedMessages,
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
