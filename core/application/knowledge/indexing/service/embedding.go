package service

import (
	"context"
	"fmt"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

func (service *IndexingService) embedChunks(ctx context.Context, job *domainknowledge.IndexingJob, document *domainknowledge.Document, profiles loadedProfiles, provider knowledgeport.EmbeddingProvider) error {
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
			return fmt.Errorf("list chunks for embedding: %w", err)
		}
		if len(chunks) == 0 {
			break
		}
		if err := validateChunkPage(chunks, expectedOrdinal, job.TotalChunks); err != nil {
			return err
		}

		missing := chunksMissingEmbedding(chunks, profiles.embedding.ID)
		if len(missing) > 0 {
			request := embedRequest(missing, profiles.embedding.ID)
			result, err := provider.EmbedDocuments(ctx, request)
			if err != nil {
				return fmt.Errorf("embed document chunks: %w", err)
			}
			vectors, chunkIDs, err := service.embeddingVectors(result, missing, *document, profiles.embedding)
			if err != nil {
				return err
			}
			if err := service.configuration.Vectors.Upsert(ctx, vectors); err != nil {
				return fmt.Errorf("upsert document vectors: %w", err)
			}
			if err := service.configuration.Chunks.MarkEmbedded(ctx, chunkIDs, profiles.embedding.ID, service.now()); err != nil {
				return fmt.Errorf("mark document chunks embedded: %w", err)
			}
		}

		afterOrdinal = chunks[len(chunks)-1].Ordinal
		expectedOrdinal = afterOrdinal + 1
		job.CompletedChunks = expectedOrdinal
		if err := service.saveJob(ctx, job); err != nil {
			return err
		}
	}
	if expectedOrdinal != job.TotalChunks {
		return fmt.Errorf("loaded %d chunks for embedding, expected %d", expectedOrdinal, job.TotalChunks)
	}
	return nil
}

func embedRequest(chunks []domainknowledge.Chunk, embeddingProfileID string) domainknowledge.EmbedRequest {
	inputs := make([]domainknowledge.EmbedInput, 0, len(chunks))
	for _, chunk := range chunks {
		inputs = append(inputs, domainknowledge.EmbedInput{ID: chunk.ID, Text: chunk.Text})
	}
	return domainknowledge.EmbedRequest{EmbeddingProfileID: embeddingProfileID, Inputs: inputs}
}

func (service *IndexingService) embeddingVectors(result domainknowledge.EmbedResult, chunks []domainknowledge.Chunk, document domainknowledge.Document, profile domainknowledge.EmbeddingProfile) ([]domainknowledge.EmbeddingVector, []string, error) {
	if err := result.Validate(); err != nil {
		return nil, nil, fmt.Errorf("validate embedding result: %w", err)
	}
	if result.EmbeddingProfileID != profile.ID {
		return nil, nil, fmt.Errorf("embedding result profile %q does not match %q", result.EmbeddingProfileID, profile.ID)
	}
	if result.Dimensions != profile.Dimensions {
		return nil, nil, fmt.Errorf("embedding result dimensions %d do not match profile dimensions %d", result.Dimensions, profile.Dimensions)
	}
	if len(result.Outputs) != len(chunks) {
		return nil, nil, fmt.Errorf("embedding returned %d outputs for %d chunks", len(result.Outputs), len(chunks))
	}

	outputs := make(map[string]domainknowledge.EmbedOutput, len(result.Outputs))
	for _, output := range result.Outputs {
		if _, exists := outputs[output.ID]; exists {
			return nil, nil, fmt.Errorf("embedding output id %q is duplicated", output.ID)
		}
		outputs[output.ID] = output
	}

	vectors := make([]domainknowledge.EmbeddingVector, 0, len(chunks))
	chunkIDs := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		output, exists := outputs[chunk.ID]
		if !exists {
			return nil, nil, fmt.Errorf("embedding output for chunk %q is missing", chunk.ID)
		}
		vector := domainknowledge.EmbeddingVector{
			ID:                 service.configuration.StableIDs.EmbeddingID(chunk.ID, profile.ID),
			ChunkID:            chunk.ID,
			KnowledgeBaseID:    chunk.KnowledgeBaseID,
			DocumentID:         chunk.DocumentID,
			DocumentVersion:    chunk.DocumentVersion,
			EmbeddingProfileID: profile.ID,
			Dimensions:         result.Dimensions,
			Values:             append([]float32(nil), output.Vector...),
			SyncSequence:       document.SyncSequence,
		}
		if err := vector.Validate(); err != nil {
			return nil, nil, fmt.Errorf("build vector for chunk %q: %w", chunk.ID, err)
		}
		vectors = append(vectors, vector)
		chunkIDs = append(chunkIDs, chunk.ID)
	}
	return vectors, chunkIDs, nil
}

func chunksMissingEmbedding(chunks []domainknowledge.Chunk, embeddingProfileID string) []domainknowledge.Chunk {
	missing := make([]domainknowledge.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		found := false
		for _, profileID := range chunk.EmbeddingProfileIDs {
			if profileID == embeddingProfileID {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, chunk)
		}
	}
	return missing
}

func validateChunkPage(chunks []domainknowledge.Chunk, expectedOrdinal int, totalChunks int) error {
	for _, chunk := range chunks {
		if err := chunk.Validate(); err != nil {
			return fmt.Errorf("validate chunk %q: %w", chunk.ID, err)
		}
		if chunk.Ordinal != expectedOrdinal {
			return fmt.Errorf("chunk ordinal %d, expected %d", chunk.Ordinal, expectedOrdinal)
		}
		if chunk.Ordinal >= totalChunks {
			return fmt.Errorf("chunk ordinal %d exceeds processor chunk count %d", chunk.Ordinal, totalChunks)
		}
		expectedOrdinal++
	}
	return nil
}
