package service

import (
	"context"
	"fmt"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

func (service *RetrievalService) fillLocalCache(
	ctx context.Context,
	query domainknowledge.RetrievalQuery,
	indexProfile domainknowledge.IndexProfile,
	embeddingProfile domainknowledge.EmbeddingProfile,
	provider knowledgeport.EmbeddingProvider,
	local localSearchState,
	remoteHits []domainknowledge.VectorHit,
	finalHits []domainknowledge.RetrievalHit,
	chunks map[string]domainknowledge.Chunk,
) (int, []string) {
	warnings := make([]string, 0)
	remoteByChunk := make(map[string]domainknowledge.VectorHit, len(remoteHits))
	for _, hit := range remoteHits {
		if _, exists := remoteByChunk[hit.ChunkID]; !exists {
			remoteByChunk[hit.ChunkID] = hit
		}
	}
	localVectorChunks := make(map[string]struct{}, len(local.vectorHits))
	for _, hit := range local.vectorHits {
		localVectorChunks[hit.ChunkID] = struct{}{}
	}
	localKeywordChunks := make(map[string]struct{}, len(local.keywordHits))
	for _, hit := range local.keywordHits {
		localKeywordChunks[hit.ChunkID] = struct{}{}
	}
	selected := make([]domainknowledge.Chunk, 0, len(finalHits))
	for _, hit := range finalHits {
		if _, remote := remoteByChunk[hit.ChunkID]; !remote {
			continue
		}
		chunk, exists := chunks[hit.ChunkID]
		if exists {
			selected = append(selected, chunk)
		}
	}
	if len(selected) == 0 {
		return 0, nil
	}
	filled := make(map[string]struct{}, len(selected))
	if service.configuration.LocalKeywords != nil {
		documents := make([]domainknowledge.KeywordDocument, 0, len(selected))
		for _, chunk := range selected {
			if _, exists := localKeywordChunks[chunk.ID]; exists {
				continue
			}
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
		if len(documents) > 0 {
			if err := service.configuration.LocalKeywords.Upsert(ctx, documents); err != nil {
				warnings = append(warnings, fmt.Sprintf("fill local keyword cache: %v", err))
			} else {
				for _, document := range documents {
					filled[document.ChunkID] = struct{}{}
				}
			}
		}
	}
	if service.configuration.LocalVectors == nil || provider == nil {
		return len(filled), warnings
	}
	inputs := make([]domainknowledge.EmbedInput, 0, len(selected))
	vectorChunks := make([]domainknowledge.Chunk, 0, len(selected))
	for _, chunk := range selected {
		if _, exists := localVectorChunks[chunk.ID]; exists {
			continue
		}
		if remoteByChunk[chunk.ID].EmbeddingID == "" {
			continue
		}
		inputs = append(inputs, domainknowledge.EmbedInput{ID: chunk.ID, Text: chunk.Text})
		vectorChunks = append(vectorChunks, chunk)
	}
	if len(inputs) == 0 {
		return len(filled), warnings
	}
	if err := service.configuration.LocalVectors.EnsureIndex(ctx, domainknowledge.VectorIndexDefinition{
		EmbeddingProfileID: embeddingProfile.ID,
		Dimensions:         embeddingProfile.Dimensions,
		DistanceMetricID:   indexProfile.DistanceMetricID,
		Options:            indexProfile.VectorIndexOptions,
	}); err != nil {
		warnings = append(warnings, fmt.Sprintf("prepare local vector cache: %v", err))
		return len(filled), warnings
	}
	embedded, err := provider.EmbedDocuments(ctx, domainknowledge.EmbedRequest{
		EmbeddingProfileID: query.EmbeddingProfileID,
		Inputs:             inputs,
	})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("embed remote cache chunks: %v", err))
		return len(filled), warnings
	}
	if err := embedded.Validate(); err != nil {
		warnings = append(warnings, fmt.Sprintf("validate remote cache embeddings: %v", err))
		return len(filled), warnings
	}
	if embedded.EmbeddingProfileID != embeddingProfile.ID || embedded.Dimensions != embeddingProfile.Dimensions {
		warnings = append(warnings, fmt.Sprintf("remote cache embeddings do not match profile %q", embeddingProfile.ID))
		return len(filled), warnings
	}
	outputs := make(map[string][]float32, len(embedded.Outputs))
	for _, output := range embedded.Outputs {
		outputs[output.ID] = output.Vector
	}
	vectors := make([]domainknowledge.EmbeddingVector, 0, len(vectorChunks))
	for _, chunk := range vectorChunks {
		values, exists := outputs[chunk.ID]
		if !exists {
			warnings = append(warnings, fmt.Sprintf("embedding output for cache chunk %q is missing", chunk.ID))
			continue
		}
		vectors = append(vectors, domainknowledge.EmbeddingVector{
			ID:                 remoteByChunk[chunk.ID].EmbeddingID,
			ChunkID:            chunk.ID,
			KnowledgeBaseID:    chunk.KnowledgeBaseID,
			DocumentID:         chunk.DocumentID,
			DocumentVersion:    chunk.DocumentVersion,
			EmbeddingProfileID: embeddingProfile.ID,
			Dimensions:         embedded.Dimensions,
			Values:             append([]float32(nil), values...),
			Deletion:           chunk.Deletion,
			SyncSequence:       chunk.SyncSequence,
		})
	}
	if len(vectors) == 0 {
		return len(filled), warnings
	}
	if err := service.configuration.LocalVectors.Upsert(ctx, vectors); err != nil {
		warnings = append(warnings, fmt.Sprintf("fill local vector cache: %v", err))
		return len(filled), warnings
	}
	for _, vector := range vectors {
		filled[vector.ChunkID] = struct{}{}
	}
	return len(filled), warnings
}
