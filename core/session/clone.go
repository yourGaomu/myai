package session

import (
	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
)

// Clone creates a point-in-time Session snapshot. The returned value shares no
// mutable slices or pointer-backed domain values with the source Session.
func Clone(current *Session) *Session {
	if current == nil {
		return nil
	}
	cloned := *current
	cloned.AllowedTools = append([]string(nil), current.AllowedTools...)
	cloned.CurrentPlan = agentplan.Clone(current.CurrentPlan)
	cloned.RAGSettings = CloneRAGSettings(current.RAGSettings)
	cloned.GenerationSettings = generation.Clone(current.GenerationSettings)
	if current.CompactionCheckpoint != nil {
		cloned.CompactionCheckpoint = current.CompactionCheckpoint.Clone()
	}
	cloned.Messages = domainmessage.CloneAll(current.Messages)
	return &cloned
}
