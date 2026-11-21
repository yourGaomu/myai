package model

type TokenUsage struct {
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	ReasoningTokens    int
	Available          bool
	PromptCachedTokens int
	// PromptCacheReported is true when at least one provider response explicitly reported a cache count, including zero.
	PromptCacheReported bool
	// PromptCacheIncomplete is true when at least one request did not report a cache count.
	PromptCacheIncomplete bool
}

// HasPromptCacheCount also recognizes positive counts stored before reporting was tracked.
func (u TokenUsage) HasPromptCacheCount() bool {
	return u.PromptCacheReported || u.PromptCachedTokens > 0
}

// AsSample records that a request took place even when the provider returned no usage at all.
// The unmarked zero value remains the identity for Add.
func (u TokenUsage) AsSample() TokenUsage {
	u.PromptCacheIncomplete = u.PromptCacheIncomplete || !u.HasPromptCacheCount()
	return u
}

func (u TokenUsage) Add(next TokenUsage) TokenUsage {
	if u == (TokenUsage{}) {
		return next
	}
	if next == (TokenUsage{}) {
		return u
	}
	return TokenUsage{
		PromptTokens:          u.PromptTokens + next.PromptTokens,
		CompletionTokens:      u.CompletionTokens + next.CompletionTokens,
		TotalTokens:           u.TotalTokens + next.TotalTokens,
		ReasoningTokens:       u.ReasoningTokens + next.ReasoningTokens,
		Available:             u.Available || next.Available,
		PromptCachedTokens:    u.PromptCachedTokens + next.PromptCachedTokens,
		PromptCacheReported:   u.PromptCacheReported || next.PromptCacheReported,
		PromptCacheIncomplete: u.cacheIncomplete() || next.cacheIncomplete(),
	}
}

func (u TokenUsage) cacheIncomplete() bool {
	return u.PromptCacheIncomplete || (u != TokenUsage{} && !u.HasPromptCacheCount())
}
