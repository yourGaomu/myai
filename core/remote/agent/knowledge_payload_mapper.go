package agent

import (
	chatretrievalresult "myai/core/application/chat/retrieval/result"
	catalogresult "myai/core/application/knowledge/catalog/result"
	queryresult "myai/core/application/knowledge/query/result"
	searchresult "myai/core/application/knowledge/search/result"
	"myai/core/remote/protocol"
)

func knowledgeCatalogPayload(result catalogresult.Catalog, message string) protocol.KnowledgeCatalogResultPayload {
	payload := protocol.KnowledgeCatalogResultPayload{
		Categories:     make([]protocol.KnowledgeCategory, 0, len(result.Categories)),
		KnowledgeBases: make([]protocol.KnowledgeBase, 0, len(result.KnowledgeBases)),
		Message:        message,
	}
	for _, category := range result.Categories {
		payload.Categories = append(payload.Categories, protocol.KnowledgeCategory{
			ID: category.ID, Name: category.Name, ParentID: category.ParentID,
			AncestorIDs: append([]string(nil), category.AncestorIDs...), SortOrder: category.SortOrder,
			Deleted: category.Deletion.Deleted, DeletedAt: category.Deletion.DeletedAt,
			CreatedAt: category.CreatedAt, UpdatedAt: category.UpdatedAt,
		})
	}
	for _, base := range result.KnowledgeBases {
		payload.KnowledgeBases = append(payload.KnowledgeBases, protocol.KnowledgeBase{
			ID: base.ID, CategoryID: base.CategoryID, Name: base.Name, Description: base.Description,
			RAGEnabled: base.RAGEnabled, ActiveIndexProfileID: base.ActiveIndexProfileID,
			PendingIndexProfileID: base.PendingIndexProfileID, Deleted: base.Deletion.Deleted,
			DeletedAt: base.Deletion.DeletedAt, CreatedAt: base.CreatedAt, UpdatedAt: base.UpdatedAt,
		})
	}
	return payload
}

func chatRetrievalPayload(result chatretrievalresult.Context) *protocol.KnowledgeSearchPreviewResultPayload {
	if !result.Triggered && result.Error == "" && len(result.Search.Hits) == 0 {
		return nil
	}
	payload := knowledgeSearchPayload(result.Query, result.Search)
	payload.Error = result.Error
	return &payload
}

func knowledgeDocumentsPayload(knowledgeBaseID string, result queryresult.Documents) protocol.KnowledgeDocumentListResultPayload {
	payload := protocol.KnowledgeDocumentListResultPayload{
		KnowledgeBaseID: knowledgeBaseID,
		Documents:       make([]protocol.KnowledgeDocument, 0, len(result.Documents)),
		Jobs:            make([]protocol.KnowledgeIndexingJob, 0, len(result.Jobs)),
	}
	for _, document := range result.Documents {
		payload.Documents = append(payload.Documents, protocol.KnowledgeDocument{
			ID: document.ID, KnowledgeBaseID: document.KnowledgeBaseID, FileName: document.FileName,
			ContentType: document.ContentType, Version: document.Version, Status: string(document.Status),
			FailureReason: document.FailureReason, Deleted: document.Deletion.Deleted,
			DeletedAt: document.Deletion.DeletedAt, CreatedAt: document.CreatedAt, UpdatedAt: document.UpdatedAt,
		})
	}
	for _, job := range result.Jobs {
		payload.Jobs = append(payload.Jobs, protocol.KnowledgeIndexingJob{
			ID: job.ID, KnowledgeBaseID: job.KnowledgeBaseID, DocumentID: job.DocumentID,
			IndexProfileID: job.IndexProfileID, Stage: string(job.Stage), Status: string(job.Status),
			TotalChunks: job.TotalChunks, CompletedChunks: job.CompletedChunks, FailedChunks: job.FailedChunks,
			LastError: job.LastError, RetryCount: job.RetryCount, CreatedAt: job.CreatedAt,
			UpdatedAt: job.UpdatedAt, CompletedAt: job.CompletedAt,
		})
	}
	return payload
}

func knowledgeProfilesPayload(result queryresult.IndexProfiles) protocol.KnowledgeProfileListResultPayload {
	profiles := make([]protocol.KnowledgeIndexProfile, 0, len(result.Profiles))
	for _, profile := range result.Profiles {
		profiles = append(profiles, protocol.KnowledgeIndexProfile{
			ID: profile.ID, Name: profile.Name, ParsingProfileID: profile.ParsingProfileID,
			ChunkingProfileID: profile.ChunkingProfileID, EmbeddingProfileID: profile.EmbeddingProfileID,
			DistanceMetricID: profile.DistanceMetricID, Status: string(profile.Status),
			FailureReason: profile.FailureReason, Deleted: profile.Deletion.Deleted,
			DeletedAt: profile.Deletion.DeletedAt, CreatedAt: profile.CreatedAt, UpdatedAt: profile.UpdatedAt,
		})
	}
	return protocol.KnowledgeProfileListResultPayload{Profiles: profiles}
}

func knowledgeSearchPayload(query string, result searchresult.Search) protocol.KnowledgeSearchPreviewResultPayload {
	payload := protocol.KnowledgeSearchPreviewResultPayload{
		Query: query, Hits: make([]protocol.KnowledgeSearchHit, 0, len(result.Hits)),
		ResolvedKnowledgeBaseIDs: append([]string(nil), result.Diagnostics.ResolvedKnowledgeBaseIDs...),
		Profiles:                 make([]protocol.KnowledgeSearchProfileDiagnostic, 0, len(result.Diagnostics.ProfileSearches)),
		Warnings:                 append([]string(nil), result.Diagnostics.Warnings...),
	}
	for _, hit := range result.Hits {
		payload.Hits = append(payload.Hits, protocol.KnowledgeSearchHit{
			KnowledgeBaseID: hit.KnowledgeBaseID, DocumentID: hit.DocumentID, ChunkID: hit.ChunkID,
			Text: hit.Text, SourceName: hit.SourceName, SourceLocation: hit.SourceLocation,
			Score: hit.Score, Rank: hit.Rank, Channel: string(hit.Channel), Origin: string(hit.Origin),
		})
	}
	for _, profile := range result.Diagnostics.ProfileSearches {
		payload.Profiles = append(payload.Profiles, protocol.KnowledgeSearchProfileDiagnostic{
			IndexProfileID: profile.IndexProfileID, EmbeddingProfileID: profile.EmbeddingProfileID,
			KnowledgeBaseIDs: append([]string(nil), profile.KnowledgeBaseIDs...),
			LocalVectorHits:  profile.Diagnostics.LocalVectorHits,
			LocalKeywordHits: profile.Diagnostics.LocalKeywordHits,
			RemoteVectorHits: profile.Diagnostics.RemoteVectorHits,
			RemoteFallback:   profile.Diagnostics.RemoteFallback,
			CacheFillCount:   profile.Diagnostics.CacheFillCount, Error: profile.Error,
		})
	}
	return payload
}
