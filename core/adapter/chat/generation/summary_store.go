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

func (s SummaryStore) SaveSummary(ctx context.Context, current *session.Session, summary string, compactedMessages int) error {
	if current == nil {
		return errors.New("session is nil")
	}
	if s.Memory == nil {
		return errors.New("session manager is nil")
	}
	if err := s.Memory.SetSummaryForSession(current.ID, summary, compactedMessages); err != nil {
		return err
	}
	current.Summary = summary
	current.CompactedMessages = compactedMessages
	current.CompactionSourceHash = ""
	current.CompactionCheckpoint = nil
	if s.Sessions == nil {
		return nil
	}
	return s.Sessions.Save(ctx, sessioncommand.SaveSession{SessionID: current.ID, Model: current.Model})
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
	if checkpointMemory, ok := s.Memory.(SummaryMemoryWithCheckpointObject); ok {
		if err := checkpointMemory.SetSummaryForSessionWithCheckpointObject(current.ID, summary, compactedMessages, sourceHash, &checkpoint); err != nil {
			return err
		}
	} else if checkpointMemory, ok := s.Memory.(SummaryMemoryWithCheckpoint); ok {
		if err := checkpointMemory.SetSummaryForSessionWithCheckpoint(current.ID, summary, compactedMessages, sourceHash); err != nil {
			return err
		}
	} else {
		if err := s.Memory.SetSummaryForSession(current.ID, summary, compactedMessages); err != nil {
			return err
		}
		current.CompactionSourceHash = sourceHash
	}
	current.CompactionSourceHash = sourceHash
	current.CompactionCheckpoint = checkpoint.Clone()
	if s.Sessions == nil {
		return nil
	}
	return s.Sessions.Save(ctx, sessioncommand.SaveSession{
		SessionID: current.ID,
		Model:     current.Model,
	})
}
