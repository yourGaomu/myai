package repository

type TokenUsageRecord struct {
	PromptTokens          int
	CompletionTokens      int
	TotalTokens           int
	ReasoningTokens       int
	PromptCachedTokens    int
	PromptCacheReported   bool
	PromptCacheIncomplete bool
	Available             bool
}
