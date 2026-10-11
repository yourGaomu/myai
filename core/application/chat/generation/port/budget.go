package port

import (
	"context"
	compactionresult "myai/core/application/chat/compaction/result"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type BudgetedAutoCompactor interface {
	CompactIfNeededWithBudget(ctx context.Context, current *session.Session, model modelport.ChatModelPort, reservedTokens int) (compactionresult.CompactInfo, error)
}
