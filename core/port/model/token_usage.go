package model

type TokenUsage struct {
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	ReasoningTokens    int
	Available          bool
	PromptCachedTokens int
}

func (u TokenUsage) Add(next TokenUsage) TokenUsage {
	return TokenUsage{
		PromptTokens:       u.PromptTokens + next.PromptTokens,
		CompletionTokens:   u.CompletionTokens + next.CompletionTokens,
		TotalTokens:        u.TotalTokens + next.TotalTokens,
		ReasoningTokens:    u.ReasoningTokens + next.ReasoningTokens,
		Available:          u.Available || next.Available,
		PromptCachedTokens: u.PromptCachedTokens + next.PromptCachedTokens,
	}
}
