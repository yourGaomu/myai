package service

import (
	"context"
	"errors"
	"fmt"

	compactionapi "myai/core/application/chat/compaction/api"
	compactionport "myai/core/application/chat/compaction/port"
	compactionresult "myai/core/application/chat/compaction/result"
	chatcontextport "myai/core/application/chat/context/port"
	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

const DefaultKeepChunks = 8

var ErrNotEnoughHistory = errors.New("not enough new history to compact")

type CompactService struct {
	Contexts   chatcontextport.Provider
	Summarizer compactionport.SummaryGenerator
	Summaries  compactionport.SummaryStore
	KeepChunks int
}

var _ compactionapi.Compactor = CompactService{}
var _ compactionport.RequestCompactor = CompactService{}

func (s CompactService) validate(current *session.Session, model modelport.ChatModelPort) error {
	if current == nil {
		return errors.New("session is nil")
	}
	if model == nil {
		return errors.New("model is nil")
	}
	if s.Summarizer == nil {
		return errors.New("summary generator is nil")
	}
	if s.Summaries == nil {
		return errors.New("compact summary store is nil")
	}
	return contextmgr.ValidateToolPairs(current.Messages)
}

func sessionCheckpoint(current *session.Session) compaction.Checkpoint {
	checkpoint := compaction.LegacyCheckpoint(current.Summary, current.CompactedMessages, current.CompactionSourceHash)
	if current.CompactionCheckpoint != nil {
		checkpoint = *current.CompactionCheckpoint.Clone()
	}
	return contextmgr.CheckpointForRequest(current.Messages, checkpoint)
}

func (s CompactService) CompactSession(ctx context.Context, current *session.Session, model modelport.ChatModelPort) error {
	if err := s.validate(current, model); err != nil {
		return err
	}
	checkpoint := sessionCheckpoint(current)
	options := contextmgr.RequestOptions{Budget: contextmgr.NewRequestBudget(contextmgr.ResolveWindowTokens(current.ContextWindowK, 0), 0, 2048)}
	compactable, _, cutoff, err := contextmgr.CompactSplitForRequest(current.Messages, checkpoint.SourceEndMessage, options, s.keepChunks())
	if err != nil {
		return err
	}
	if len(compactable) == 0 {
		return ErrNotEnoughHistory
	}
	_, err = s.summarizeAndSave(ctx, current, model, checkpoint.Summary, compactable, cutoff)
	return err
}

func (s CompactService) summarizeAndSave(ctx context.Context, current *session.Session, model modelport.ChatModelPort, previous string, messages []domainmessage.Message, cutoff int) (string, error) {
	summary, err := s.Summarizer.Summarize(ctx, model, previous, messages)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	hash := contextmgr.CompactionSourceHash(current.Messages, cutoff)
	if checkpointStore, ok := s.Summaries.(compactionport.CheckpointSummaryStore); ok {
		err = checkpointStore.SaveSummaryWithCheckpoint(ctx, current, summary, cutoff, hash)
	} else {
		err = s.Summaries.SaveSummary(ctx, current, summary, cutoff)
	}
	return summary, err
}

func (s CompactService) CompactIfNeeded(ctx context.Context, current *session.Session, model modelport.ChatModelPort) (compactionresult.CompactInfo, error) {
	return s.CompactIfNeededWithBudget(ctx, current, model, 0)
}

func (s CompactService) CompactIfNeededWithBudget(ctx context.Context, current *session.Session, model modelport.ChatModelPort, reservedTokens int) (compactionresult.CompactInfo, error) {
	if current == nil || model == nil {
		return compactionresult.CompactInfo{}, nil
	}
	budget := contextmgr.RequestBudget{WindowTokens: contextmgr.ResolveWindowTokens(current.ContextWindowK, 0), SafetyTokens: max(0, reservedTokens)}
	return s.CompactIfNeededForRequest(ctx, current, model, contextmgr.RequestOptions{Budget: budget})
}

func (s CompactService) CompactIfNeededForRequest(ctx context.Context, current *session.Session, model modelport.ChatModelPort, options contextmgr.RequestOptions) (compactionresult.CompactInfo, error) {
	if current == nil || model == nil {
		return compactionresult.CompactInfo{}, nil
	}
	checkpoint := sessionCheckpoint(current)
	before := contextmgr.BuildSnapshotForRequest(current.Messages, checkpoint, checkpoint.SourceEndMessage, options).Info
	budget := options.Budget
	if budget.WindowTokens <= 0 {
		budget.WindowTokens = contextmgr.ResolveWindowTokens(current.ContextWindowK, 0)
		options.Budget = budget
	}
	if !before.Truncated && before.SelectedTokens+budget.ReservedTokens() < budget.WindowTokens*7/10 {
		return compactionresult.CompactInfo{}, nil
	}
	info := compactionresult.CompactInfo{Triggered: true, Reason: compactReason(before), BeforeTokens: before.SelectedTokens}
	fail := func(err error) (compactionresult.CompactInfo, error) {
		info.Failed, info.Error = true, err.Error()
		info.Truncated, info.DroppedMessages = before.Truncated, before.DroppedMessages
		return info, err
	}
	if err := s.validate(current, model); err != nil {
		return fail(err)
	}
	compactable, _, cutoff, err := contextmgr.CompactSplitForRequest(current.Messages, checkpoint.SourceEndMessage, options, s.keepChunks())
	if err != nil {
		return fail(err)
	}
	if len(compactable) == 0 {
		if before.Truncated {
			return fail(fmt.Errorf("context compaction cannot preserve the active request: %w", ErrNotEnoughHistory))
		}
		return compactionresult.CompactInfo{}, nil
	}
	summary, err := s.summarizeAndSave(ctx, current, model, checkpoint.Summary, compactable, cutoff)
	if err != nil {
		return fail(err)
	}
	// SummaryStore owns the authoritative write. Measure the newly persisted
	// state independently even when the store does not mutate its input snapshot.
	next := compaction.LegacyCheckpoint(summary, cutoff, contextmgr.CompactionSourceHash(current.Messages, cutoff))
	after := contextmgr.BuildSnapshotForRequest(current.Messages, next, cutoff, options).Info
	info.AfterTokens, info.NewMessages = after.SelectedTokens, after.CompactedMessages-before.CompactedMessages
	info.CompactedMessages, info.SummaryTokens, info.SummaryVersion = after.CompactedMessages, after.SummaryTokens, after.SummaryVersion
	info.SummaryHash, info.PrefixHash, info.CacheableTokens, info.Checkpoint = after.SummaryHash, after.PrefixHash, after.CacheableTokens, after.Checkpoint
	info.Truncated, info.DroppedMessages = after.Truncated || after.SummaryTruncated, after.DroppedMessages
	if after.DroppedMessages > 0 || after.SelectedTokens > budget.MessageLimit() {
		return fail(errors.New("compaction completed but the request still exceeds the context budget; history was retained"))
	}
	return info, nil
}

func (s CompactService) keepChunks() int {
	if s.KeepChunks > 0 {
		return s.KeepChunks
	}
	return DefaultKeepChunks
}

func compactReason(info contextmgr.Info) string {
	if info.Truncated {
		return "window_limit"
	}
	return "threshold"
}
