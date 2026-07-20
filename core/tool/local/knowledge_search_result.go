package local

import (
	searchresult "myai/core/application/knowledge/search/result"
	domainknowledge "myai/core/domain/knowledge"
)

type knowledgeSearchResult struct {
	Query       string                     `json:"query"`
	Count       int                        `json:"count"`
	Hits        []knowledgeSearchHit       `json:"hits"`
	Diagnostics knowledgeSearchDiagnostics `json:"diagnostics"`
}

type knowledgeSearchHit struct {
	KnowledgeBaseID string  `json:"knowledge_base_id"`
	DocumentID      string  `json:"document_id"`
	ChunkID         string  `json:"chunk_id"`
	DocumentVersion int64   `json:"document_version"`
	Text            string  `json:"text"`
	SourceName      string  `json:"source_name"`
	SourceLocation  string  `json:"source_location"`
	Score           float64 `json:"score"`
	Rank            int     `json:"rank"`
	Channel         string  `json:"channel"`
	Origin          string  `json:"origin"`
}

type knowledgeSearchDiagnostics struct {
	ResolvedKnowledgeBaseIDs []string                           `json:"resolved_knowledge_base_ids"`
	LocalVectorHits          int                                `json:"local_vector_hits"`
	LocalKeywordHits         int                                `json:"local_keyword_hits"`
	RemoteVectorHits         int                                `json:"remote_vector_hits"`
	RemoteFallback           bool                               `json:"remote_fallback"`
	CacheFillCount           int                                `json:"cache_fill_count"`
	Profiles                 []knowledgeSearchProfileDiagnostic `json:"profiles"`
	Warnings                 []string                           `json:"warnings"`
}

type knowledgeSearchProfileDiagnostic struct {
	IndexProfileID     string   `json:"index_profile_id"`
	EmbeddingProfileID string   `json:"embedding_profile_id"`
	KnowledgeBaseIDs   []string `json:"knowledge_base_ids"`
	LocalVectorHits    int      `json:"local_vector_hits"`
	LocalKeywordHits   int      `json:"local_keyword_hits"`
	RemoteVectorHits   int      `json:"remote_vector_hits"`
	RemoteFallback     bool     `json:"remote_fallback"`
	CacheFillCount     int      `json:"cache_fill_count"`
	LocalQualityPassed bool     `json:"local_quality_passed"`
	Error              string   `json:"error,omitempty"`
}

func newKnowledgeSearchResult(query string, response searchresult.Search) knowledgeSearchResult {
	hits := make([]knowledgeSearchHit, 0, len(response.Hits))
	for _, hit := range response.Hits {
		hits = append(hits, newKnowledgeSearchHit(hit))
	}

	warnings := append([]string(nil), response.Diagnostics.Warnings...)
	if warnings == nil {
		warnings = []string{}
	}
	profiles := make([]knowledgeSearchProfileDiagnostic, 0, len(response.Diagnostics.ProfileSearches))
	diagnostics := knowledgeSearchDiagnostics{
		ResolvedKnowledgeBaseIDs: append([]string(nil), response.Diagnostics.ResolvedKnowledgeBaseIDs...),
		Profiles:                 profiles,
		Warnings:                 warnings,
	}
	for _, profile := range response.Diagnostics.ProfileSearches {
		diagnostics.LocalVectorHits += profile.Diagnostics.LocalVectorHits
		diagnostics.LocalKeywordHits += profile.Diagnostics.LocalKeywordHits
		diagnostics.RemoteVectorHits += profile.Diagnostics.RemoteVectorHits
		diagnostics.CacheFillCount += profile.Diagnostics.CacheFillCount
		diagnostics.RemoteFallback = diagnostics.RemoteFallback || profile.Diagnostics.RemoteFallback
		diagnostics.Profiles = append(diagnostics.Profiles, knowledgeSearchProfileDiagnostic{
			IndexProfileID: profile.IndexProfileID, EmbeddingProfileID: profile.EmbeddingProfileID,
			KnowledgeBaseIDs: append([]string(nil), profile.KnowledgeBaseIDs...),
			LocalVectorHits:  profile.Diagnostics.LocalVectorHits, LocalKeywordHits: profile.Diagnostics.LocalKeywordHits,
			RemoteVectorHits: profile.Diagnostics.RemoteVectorHits, RemoteFallback: profile.Diagnostics.RemoteFallback,
			CacheFillCount: profile.Diagnostics.CacheFillCount, LocalQualityPassed: profile.Diagnostics.LocalQuality.Passed,
			Error: profile.Error,
		})
	}

	return knowledgeSearchResult{
		Query:       query,
		Count:       len(hits),
		Hits:        hits,
		Diagnostics: diagnostics,
	}
}

func newKnowledgeSearchHit(hit domainknowledge.RetrievalHit) knowledgeSearchHit {
	return knowledgeSearchHit{
		KnowledgeBaseID: hit.KnowledgeBaseID,
		DocumentID:      hit.DocumentID,
		ChunkID:         hit.ChunkID,
		DocumentVersion: hit.DocumentVersion,
		Text:            hit.Text,
		SourceName:      hit.SourceName,
		SourceLocation:  hit.SourceLocation,
		Score:           hit.Score,
		Rank:            hit.Rank,
		Channel:         string(hit.Channel),
		Origin:          string(hit.Origin),
	}
}
