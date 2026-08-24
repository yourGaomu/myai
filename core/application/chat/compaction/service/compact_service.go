package service

import (
	"context"
	"errors"

	compactionapi "myai/core/application/chat/compaction/api"
	compactionport "myai/core/application/chat/compaction/port"
	compactionresult "myai/core/application/chat/compaction/result"
	chatcontextport "myai/core/application/chat/context/port"
	"myai/core/contextmgr"
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

func (s CompactService) CompactSession(ctx context.Context, current *session.Session, model modelport.ChatModelPort) error {
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
	// 只摘要较旧的完整消息块，并保留最近若干块原文，兼顾上下文连续性与 token 预算。
	summary := current.Summary
	compactedMessages := current.CompactedMessages
	if current.CompactionCheckpoint != nil {
		if !contextmgr.CompactionCheckpointMatchesCheckpoint(current.Messages, current.CompactionCheckpoint) {
			summary = ""
			compactedMessages = 0
		} else {
			summary = current.CompactionCheckpoint.Summary
			compactedMessages = current.CompactionCheckpoint.SourceEndMessage
		}
	} else if !contextmgr.CompactionCheckpointMatches(current.Messages, summary, compactedMessages, current.CompactionSourceHash) {
		// Rebuild from the first user turn when the persisted checkpoint no
		// longer describes the current message prefix.
		summary = ""
		compactedMessages = 0
	}
	compactable, _, cutoff := contextmgr.CompactSplit(current.Messages, compactedMessages, s.keepChunks())
	if len(compactable) == 0 {
		return ErrNotEnoughHistory
	}
	summary, err := s.Summarizer.Summarize(ctx, model, summary, compactable)
	if err != nil {
		return err
	}
	sourceHash := contextmgr.CompactionSourceHash(current.Messages, cutoff)
	if checkpointStore, ok := s.Summaries.(compactionport.CheckpointSummaryStore); ok {
		return checkpointStore.SaveSummaryWithCheckpoint(ctx, current, summary, cutoff, sourceHash)
	}
	return s.Summaries.SaveSummary(ctx, current, summary, cutoff)
}

func (s CompactService) CompactIfNeeded(ctx context.Context, current *session.Session, model modelport.ChatModelPort) (compactionresult.CompactInfo, error) {
	if current == nil || model == nil {
		return compactionresult.CompactInfo{}, nil
	}
	if s.Contexts == nil {
		return compactionresult.CompactInfo{}, errors.New("context provider is nil")
	}
	before := s.Contexts.Snapshot(current).Info
	if !contextmgr.ShouldCompact(before, contextmgr.DefaultCompactTriggerRatio) {
		return compactionresult.CompactInfo{}, nil
	}
	if err := s.CompactSession(ctx, current, model); errors.Is(err, ErrNotEnoughHistory) {
		return compactionresult.CompactInfo{}, nil
	} else {
		after := s.Contexts.Snapshot(current).Info
		return compactionresult.CompactInfo{Triggered: true, Reason: compactReason(before), BeforeTokens: before.SelectedTokens, AfterTokens: after.SelectedTokens, NewMessages: after.CompactedMessages - before.CompactedMessages, CompactedMessages: after.CompactedMessages, SummaryTokens: after.SummaryTokens, SummaryVersion: after.SummaryVersion, SummaryHash: after.SummaryHash, PrefixHash: after.PrefixHash, CacheableTokens: after.CacheableTokens, Checkpoint: after.Checkpoint}, err
	}
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
