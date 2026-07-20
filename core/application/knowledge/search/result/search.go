package result

import (
	retrievalresult "myai/core/application/knowledge/retrieval/result"
	domainknowledge "myai/core/domain/knowledge"
)

type ProfileSearch struct {
	IndexProfileID     string
	EmbeddingProfileID string
	KnowledgeBaseIDs   []string
	Diagnostics        retrievalresult.Diagnostics
	Error              string
}

type Diagnostics struct {
	ResolvedKnowledgeBaseIDs []string
	ProfileSearches          []ProfileSearch
	Warnings                 []string
}

type Search struct {
	Hits        []domainknowledge.RetrievalHit
	Diagnostics Diagnostics
}
