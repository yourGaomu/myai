package generation

import (
	"context"
	"errors"
	"strings"
	"time"

	sessioncommand "myai/core/application/session/command"
	compaction "myai/core/domain/compaction"
	"myai/core/session"
)

type SummaryMemory interface {
	SetSummaryForSession(sessionID string, summary string, compactedMessages int) error
}

type SummaryMemoryWithCheckpoint interface {
	SetSummaryForSessionWithCheckpoint(sessionID string, summary string, compactedMessages int, sourceHash string) error
}

type SummaryMemoryWithCheckpointObject interface {
	SetSummaryForSessionWithCheckpointObject(sessionID string, summary string, compactedMessages int, sourceHash string, checkpoint *compaction.Checkpoint) error
}

type SummaryStore struct {
	Memory   SummaryMemory
	Sessions SummaryPersistence
}

type summaryState struct {
	summary           string
	compactedMessages int
	sourceHash        string
	checkpoint        *compaction.Checkpoint
}

func (s SummaryStore) SaveSummary(ctx context.Context, current *session.Session, summary string, compactedMessages int) error {
	if current == nil {
		return errors.New("session is nil")
	}
	if s.Memory == nil {
		return errors.New("session manager is nil")
	}
	previous := captureSummaryState(current)
	if err := s.setMemorySummary(current.ID, summary, compactedMessages, "", nil); err != nil {
		return err
	}
	applySummaryState(current, summaryState{summary: summary, compactedMessages: compactedMessages})
	if s.Sessions == nil {
		return nil
	}
	if err := s.Sessions.Save(ctx, sessioncommand.SaveSession{SessionID: current.ID, Model: current.Model}); err != nil {
		return errors.Join(err, s.rollbackSummary(current, previous))
	}
	return nil
}

func (s SummaryStore) SaveSummaryWithCheckpoint(ctx context.Context, current *session.Session, summary string, compactedMessages int, sourceHash string) error {
	if current == nil {
		return errors.New("session is nil")
	}
	if s.Memory == nil {
		return errors.New("session manager is nil")
	}
	checkpoint := compaction.LegacyCheckpoint(summary, compactedMessages, sourceHash)
	if strings.TrimSpace(sourceHash) != "" {
		var checkpointErr error
		checkpoint, checkpointErr = compaction.NewCheckpoint(summary, 0, compactedMessages, sourceHash, time.Now())
		if checkpointErr != nil {
			return checkpointErr
		}
	}
	previous := captureSummaryState(current)
	if err := s.setMemorySummary(current.ID, summary, compactedMessages, sourceHash, &checkpoint); err != nil {
		return err
	}
	applySummaryState(current, summaryState{
		summary: summary, compactedMessages: compactedMessages,
		sourceHash: sourceHash, checkpoint: checkpoint.Clone(),
	})
	if s.Sessions == nil {
		return nil
	}
	if err := s.Sessions.Save(ctx, sessioncommand.SaveSession{
		SessionID: current.ID,
		Model:     current.Model,
	}); err != nil {
		return errors.Join(err, s.rollbackSummary(current, previous))
	}
	return nil
}

func (s SummaryStore) setMemorySummary(sessionID string, summary string, compactedMessages int, sourceHash string, checkpoint *compaction.Checkpoint) error {
	if checkpointMemory, ok := s.Memory.(SummaryMemoryWithCheckpointObject); ok {
		return checkpointMemory.SetSummaryForSessionWithCheckpointObject(sessionID, summary, compactedMessages, sourceHash, checkpoint)
	}
	if checkpointMemory, ok := s.Memory.(SummaryMemoryWithCheckpoint); ok {
		return checkpointMemory.SetSummaryForSessionWithCheckpoint(sessionID, summary, compactedMessages, sourceHash)
	}
	return s.Memory.SetSummaryForSession(sessionID, summary, compactedMessages)
}

func (s SummaryStore) rollbackSummary(current *session.Session, previous summaryState) error {
	if current == nil {
		return nil
	}
	rollbackErr := s.setMemorySummary(current.ID, previous.summary, previous.compactedMessages, previous.sourceHash, previous.checkpoint)
	applySummaryState(current, previous)
	return rollbackErr
}

func captureSummaryState(current *session.Session) summaryState {
	if current == nil {
		return summaryState{}
	}
	return summaryState{
		summary: current.Summary, compactedMessages: current.CompactedMessages,
		sourceHash: current.CompactionSourceHash, checkpoint: cloneCheckpoint(current.CompactionCheckpoint),
	}
}

func applySummaryState(current *session.Session, state summaryState) {
	if current == nil {
		return
	}
	current.Summary = state.summary
	current.CompactedMessages = state.compactedMessages
	current.CompactionSourceHash = state.sourceHash
	current.CompactionCheckpoint = cloneCheckpoint(state.checkpoint)
}

func cloneCheckpoint(checkpoint *compaction.Checkpoint) *compaction.Checkpoint {
	if checkpoint == nil {
		return nil
	}
	return checkpoint.Clone()
}
