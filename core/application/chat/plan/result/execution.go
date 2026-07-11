package result

import (
	compactionresult "myai/core/application/chat/compaction/result"
	"myai/core/contextmgr"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
)

type Execution struct {
	SessionID string
	Result    modelport.ChatResult
	Context   contextmgr.Info
	Compact   compactionresult.CompactInfo
	Plan      *agentplan.Plan
}
