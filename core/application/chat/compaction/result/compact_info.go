package result

import compaction "myai/core/domain/compaction"

type CompactInfo struct {
	Triggered         bool
	Reason            string
	BeforeTokens      int
	AfterTokens       int
	NewMessages       int
	CompactedMessages int
	SummaryTokens     int
	SummaryVersion    int
	SummaryHash       string
	PrefixHash        string
	CacheableTokens   int
	Checkpoint        *compaction.Checkpoint
}
