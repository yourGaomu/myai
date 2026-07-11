package result

type TokenUsage struct {
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	ReasoningTokens    int
	PromptCachedTokens int
	Available          bool
}
