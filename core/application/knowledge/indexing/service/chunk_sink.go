package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type chunkPersistenceSink struct {
	repository      knowledgeport.ChunkRepository
	stableIDs       knowledgeport.StableIDDeriver
	document        domainknowledge.Document
	parsingProfile  domainknowledge.ParsingProfile
	chunkingProfile domainknowledge.ChunkingProfile
	now             func() time.Time
	onFirstBatch    func(context.Context) error
	firstBatch      sync.Once
	firstBatchErr   error
	count           int
}

func (sink *chunkPersistenceSink) Accept(ctx context.Context, drafts []domainknowledge.ChunkDraft) error {
	if len(drafts) == 0 {
		return fmt.Errorf("indexing chunk batch is empty")
	}
	sink.firstBatch.Do(func() {
		if sink.onFirstBatch != nil {
			sink.firstBatchErr = sink.onFirstBatch(ctx)
		}
	})
	if sink.firstBatchErr != nil {
		return sink.firstBatchErr
	}

	now := sink.now().UTC()
	chunks := make([]domainknowledge.Chunk, 0, len(drafts))
	for _, draft := range drafts {
		identity := domainknowledge.ChunkIdentity{
			DocumentID:        sink.document.ID,
			DocumentVersion:   sink.document.Version,
			ParsingProfileID:  sink.parsingProfile.ID,
			ChunkingProfileID: sink.chunkingProfile.ID,
			Ordinal:           draft.Ordinal,
			ContentHash:       draft.ContentHash,
		}
		chunk := domainknowledge.Chunk{
			ID:                sink.stableIDs.ChunkID(identity),
			KnowledgeBaseID:   sink.document.KnowledgeBaseID,
			DocumentID:        sink.document.ID,
			DocumentVersion:   sink.document.Version,
			ParsingProfileID:  sink.parsingProfile.ID,
			ChunkingProfileID: sink.chunkingProfile.ID,
			Ordinal:           draft.Ordinal,
			Text:              draft.Text,
			ContentHash:       draft.ContentHash,
			StartOffset:       draft.StartOffset,
			EndOffset:         draft.EndOffset,
			SourcePage:        draft.SourcePage,
			SourceHeading:     draft.SourceHeading,
			SyncSequence:      sink.document.SyncSequence,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := chunk.Validate(); err != nil {
			return fmt.Errorf("build knowledge chunk %d: %w", draft.Ordinal, err)
		}
		chunks = append(chunks, chunk)
	}
	if err := sink.repository.SaveAll(ctx, chunks); err != nil {
		return err
	}
	sink.count += len(chunks)
	return nil
}
