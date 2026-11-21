package result

type TokenUsage struct {
	PromptTokens          int
	CompletionTokens      int
	TotalTokens           int
	ReasoningTokens       int
	PromptCachedTokens    int
	PromptCacheReported   bool
	PromptCacheIncomplete bool
	Available             bool
}
