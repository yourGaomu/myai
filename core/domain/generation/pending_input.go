package generation

import "time"

type PendingTurnInputItem struct {
	// 1. ID 是稳定的投递事件 ID，必须跨 drain/requeue 周期保持不变。
	ID               string
	Content          string
	SourceKind       string
	SourceTaskID     string
	AuthorAgentID    string
	RecipientAgentID string
	ParentTurnID     string
	RootAgentID      string
	Trigger          string
	Sequence         uint64
	CreatedAt        time.Time
}
