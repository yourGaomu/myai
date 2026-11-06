package contextmgr

import (
	"errors"

	compaction "myai/core/domain/compaction"
	domainmessage "myai/core/domain/message"
)

// CompactSplitForRequest selects a contiguous raw prefix, but measures its
// projected suffix with the exact request budget. Count limits are only an
// upper bound on retention; they cannot leave an unsummarized budget hole.
func CompactSplitForRequest(messages []domainmessage.Message, compactedMessages int, options RequestOptions, keepChunks int) ([]domainmessage.Message, []domainmessage.Message, int, error) {
	if err := ValidateToolPairs(messages); err != nil {
		return nil, nil, compactedMessages, err
	}
	start := 0
	if len(messages) > 0 && messages[0].Role == domainmessage.RoleSystem {
		start = 1
	}
	start = max(start, min(compactedMessages, len(messages)))
	chunks := ToolSafeChunks(messages[start:])
	if len(chunks) < 2 {
		return nil, messages[start:], start, nil
	}
	if keepChunks <= 0 {
		keepChunks = 8
	}
	budget := options.Budget
	if budget.WindowTokens <= 0 {
		budget = NewRequestBudget(DefaultWindowK*1000, budget.ToolTokens, budget.OutputTokens)
	}
	system, _ := splitSystemMessage(messages)
	// Leave room for the bounded new summary before calling the model. The
	// result is checked again after persistence, so failures never discard data.
	summaryAllowance := min(2048, budget.WindowTokens/4) + 32
	target := budget.MessageLimit() * 3 / 5
	cutoff := start
	firstFitting := 0
	for index, chunk := range chunks[:len(chunks)-1] {
		cutoff += len(chunk)
		tail, _ := requestHistory(messages, cutoff, &options.TurnContext)
		tokens := EstimateMessagesTokens(system) + EstimateMessagesTokens(tail) + summaryAllowance
		if tokens > budget.MessageLimit() {
			continue
		}
		if firstFitting == 0 {
			firstFitting = cutoff
		}
		if len(chunks)-index-1 <= keepChunks && tokens <= target {
			return messages[start:cutoff], messages[cutoff:], cutoff, nil
		}
	}
	if firstFitting > 0 {
		return messages[start:firstFitting], messages[firstFitting:], firstFitting, nil
	}
	return nil, messages[start:], start, errors.New("current request and latest complete tool exchange cannot fit the context budget")
}

// CheckpointForRequest returns an empty checkpoint for stale source history.
// Callers must use its cursor, not the stale Session.CompactedMessages value.
func CheckpointForRequest(messages []domainmessage.Message, checkpoint compaction.Checkpoint) compaction.Checkpoint {
	if !CompactionCheckpointMatchesCheckpoint(messages, &checkpoint) {
		return compaction.Checkpoint{}
	}
	return checkpoint
}
