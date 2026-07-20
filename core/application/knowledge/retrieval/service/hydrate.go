package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	domainknowledge "myai/core/domain/knowledge"
)

func (service *RetrievalService) hydrate(ctx context.Context, query domainknowledge.RetrievalQuery, indexProfile domainknowledge.IndexProfile, fused []domainknowledge.FusedItem) ([]domainknowledge.RetrievalHit, map[string]domainknowledge.Chunk, []string) {
	if len(fused) == 0 {
		return []domainknowledge.RetrievalHit{}, map[string]domainknowledge.Chunk{}, nil
	}
	chunkIDs := make([]string, 0, len(fused))
	for _, item := range fused {
		chunkIDs = append(chunkIDs, item.ChunkID)
	}
	chunks, err := service.configuration.Chunks.GetByIDs(ctx, chunkIDs)
	if err != nil {
		return nil, nil, []string{fmt.Sprintf("load retrieval chunks: %v", err)}
	}
	byID := make(map[string]domainknowledge.Chunk, len(chunks))
	for _, chunk := range chunks {
		byID[chunk.ID] = chunk
	}
	allowedKnowledgeBases := make(map[string]struct{}, len(query.KnowledgeBaseIDs))
	for _, knowledgeBaseID := range query.KnowledgeBaseIDs {
		allowedKnowledgeBases[knowledgeBaseID] = struct{}{}
	}
	documents := make(map[string]domainknowledge.Document)
	documentFailures := make(map[string]struct{})
	invalidDocuments := make(map[string]struct{})
	warnings := make([]string, 0)
	hits := make([]domainknowledge.RetrievalHit, 0, query.TopK)
	selectedChunks := make(map[string]domainknowledge.Chunk)
	for _, item := range fused {
		chunk, exists := byID[item.ChunkID]
		if !exists || chunk.Deletion.Deleted {
			continue
		}
		if err := chunk.Validate(); err != nil {
			warnings = append(warnings, fmt.Sprintf("validate retrieval chunk %q: %v", chunk.ID, err))
			continue
		}
		if len(allowedKnowledgeBases) > 0 {
			if _, allowed := allowedKnowledgeBases[chunk.KnowledgeBaseID]; !allowed {
				continue
			}
		}
		if chunk.ParsingProfileID != indexProfile.ParsingProfileID || chunk.ChunkingProfileID != indexProfile.ChunkingProfileID || !containsString(chunk.EmbeddingProfileIDs, query.EmbeddingProfileID) {
			continue
		}
		if _, invalid := invalidDocuments[chunk.DocumentID]; invalid {
			continue
		}
		document, exists := documents[chunk.DocumentID]
		if !exists {
			if _, failed := documentFailures[chunk.DocumentID]; !failed {
				loaded, err := service.configuration.Documents.Get(ctx, chunk.DocumentID)
				if err != nil {
					documentFailures[chunk.DocumentID] = struct{}{}
					warnings = append(warnings, fmt.Sprintf("load source document %q: %v", chunk.DocumentID, err))
				} else {
					if err := loaded.Validate(); err != nil || loaded.Deletion.Deleted || loaded.Status != domainknowledge.DocumentStatusReady || loaded.KnowledgeBaseID != chunk.KnowledgeBaseID || loaded.Version != chunk.DocumentVersion {
						invalidDocuments[chunk.DocumentID] = struct{}{}
						warnings = append(warnings, fmt.Sprintf("source document %q is not valid for retrieval", chunk.DocumentID))
						continue
					}
					document = loaded
					documents[chunk.DocumentID] = loaded
				}
			}
		}
		sourceName := chunk.DocumentID
		if strings.TrimSpace(document.FileName) != "" {
			sourceName = document.FileName
		}
		hits = append(hits, domainknowledge.RetrievalHit{
			KnowledgeBaseID: chunk.KnowledgeBaseID,
			DocumentID:      chunk.DocumentID,
			ChunkID:         chunk.ID,
			DocumentVersion: chunk.DocumentVersion,
			Text:            chunk.Text,
			SourceName:      sourceName,
			SourceLocation:  sourceLocation(chunk),
			Score:           item.Score,
			Rank:            len(hits) + 1,
			Channel:         item.Channel,
			Origin:          item.Origin,
		})
		selectedChunks[chunk.ID] = chunk
		if len(hits) == query.TopK {
			break
		}
	}
	return hits, selectedChunks, warnings
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func sourceLocation(chunk domainknowledge.Chunk) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(chunk.SourceHeading) != "" {
		parts = append(parts, chunk.SourceHeading)
	}
	if chunk.SourcePage > 0 {
		parts = append(parts, "page "+strconv.Itoa(chunk.SourcePage))
	}
	parts = append(parts, fmt.Sprintf("bytes %d-%d", chunk.StartOffset, chunk.EndOffset))
	return strings.Join(parts, ", ")
}
