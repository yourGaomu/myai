package generation

import "time"

const (
	// PendingTurnSourceUserMessage 用于区分排队的真实用户输入与子代理投递。
	PendingTurnSourceUserMessage = "user_message"
	// PendingTurnTriggerNextTurn 表示消息必须在当前会话操作完成后开启新一轮。
	PendingTurnTriggerNextTurn = "next_turn"
)

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
