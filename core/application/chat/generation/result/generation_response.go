package result

import (
	compactionresult "myai/core/application/chat/compaction/result"
	"myai/core/contextmgr"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
)

type GenerationResponse struct {
	SessionID string
	RunID     string
	MessageID string // History row of the committed final assistant reply, if any.
	Result    modelport.ChatResult
	Context   contextmgr.Info
	Compact   compactionresult.CompactInfo
	Plan      *agentplan.Plan
}
