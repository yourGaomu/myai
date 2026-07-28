package service

import (
	"context"
	"errors"
	"fmt"

	domainknowledge "myai/core/domain/knowledge"
)

func (service *IndexingService) saveProgress(ctx context.Context, job *domainknowledge.IndexingJob, document *domainknowledge.Document) error {
	now := service.now()
	job.UpdatedAt = now
	document.UpdatedAt = now
	if err := document.Validate(); err != nil {
		return fmt.Errorf("validate indexing document progress: %w", err)
	}
	if err := job.Validate(); err != nil {
		return fmt.Errorf("validate indexing job progress: %w", err)
	}
	if err := service.configuration.States.SaveDocumentAndJob(ctx, *document, *job); err != nil {
		return fmt.Errorf("save indexing progress: %w", err)
	}
	return nil
}

func (service *IndexingService) saveJob(ctx context.Context, job *domainknowledge.IndexingJob) error {
	job.UpdatedAt = service.now()
	if err := job.Validate(); err != nil {
		return fmt.Errorf("validate indexing job progress: %w", err)
	}
	if err := service.configuration.Jobs.Save(ctx, *job); err != nil {
		return fmt.Errorf("save indexing job progress: %w", err)
	}
	return nil
}

func (service *IndexingService) fail(ctx context.Context, job *domainknowledge.IndexingJob, document *domainknowledge.Document, cause error) error {
	if cause == nil {
		cause = fmt.Errorf("indexing failed")
	}
	persistenceContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), failurePersistenceTimeout)
	defer cancel()

	now := service.now()
	message := cause.Error()
	if document != nil {
		document.Status = domainknowledge.DocumentStatusFailed
		document.FailureReason = message
		document.UpdatedAt = now
	}
	if job != nil {
		job.Status = domainknowledge.IndexingJobStatusFailed
		job.LastError = message
		job.CompletedAt = nil
		job.UpdatedAt = now
		if job.TotalChunks > job.CompletedChunks {
			job.FailedChunks = job.TotalChunks - job.CompletedChunks
		} else {
			job.FailedChunks = 0
		}
	}

	var persistenceErr error
	switch {
	case document != nil && job != nil:
		persistenceErr = service.configuration.States.SaveDocumentAndJob(persistenceContext, *document, *job)
		if persistenceErr != nil {
			persistenceErr = fmt.Errorf("save failed indexing state: %w", persistenceErr)
		}
	case document != nil:
		persistenceErr = service.configuration.Documents.Save(persistenceContext, *document)
		if persistenceErr != nil {
			persistenceErr = fmt.Errorf("save failed document state: %w", persistenceErr)
		}
	case job != nil:
		persistenceErr = service.configuration.Jobs.Save(persistenceContext, *job)
		if persistenceErr != nil {
			persistenceErr = fmt.Errorf("save failed indexing job state: %w", persistenceErr)
		}
	}
	return errors.Join(cause, persistenceErr)
}

func (service *IndexingService) indexKeywords(ctx context.Context, document domainknowledge.Document, profiles loadedProfiles, totalChunks int) error {
	afterOrdinal := -1
	expectedOrdinal := 0
	for {
		chunks, err := service.configuration.Chunks.ListByDocumentPage(
			ctx,
			document.ID,
			document.Version,
			profiles.parsing.ID,
			profiles.chunking.ID,
			afterOrdinal,
			service.configuration.PageSize,
		)
		if err != nil {
			return fmt.Errorf("list chunks for keyword indexing: %w", err)
		}
		if len(chunks) == 0 {
			break
		}
		if err := validateChunkPage(chunks, expectedOrdinal, totalChunks); err != nil {
			return err
		}
		documents := make([]domainknowledge.KeywordDocument, 0, len(chunks))
		for _, chunk := range chunks {
			documents = append(documents, domainknowledge.KeywordDocument{
				ChunkID:         chunk.ID,
				KnowledgeBaseID: chunk.KnowledgeBaseID,
				DocumentID:      chunk.DocumentID,
				DocumentVersion: chunk.DocumentVersion,
				Text:            chunk.Text,
				Deletion:        chunk.Deletion,
				SyncSequence:    chunk.SyncSequence,
			})
		}
		if err := service.configuration.Keywords.Upsert(ctx, documents); err != nil {
			return fmt.Errorf("upsert keyword documents: %w", err)
		}
		afterOrdinal = chunks[len(chunks)-1].Ordinal
		expectedOrdinal = afterOrdinal + 1
	}
	if expectedOrdinal != totalChunks {
		return fmt.Errorf("loaded %d chunks for keyword indexing, expected %d", expectedOrdinal, totalChunks)
	}
	return nil
}
