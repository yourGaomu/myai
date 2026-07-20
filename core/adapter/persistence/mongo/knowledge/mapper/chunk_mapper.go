package mapper

import (
	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func ChunkDocumentFromDomain(chunk domainknowledge.Chunk) po.ChunkDocument {
	return po.ChunkDocument{
		ID:                  chunk.ID,
		KnowledgeBaseID:     chunk.KnowledgeBaseID,
		DocumentID:          chunk.DocumentID,
		DocumentVersion:     chunk.DocumentVersion,
		ParsingProfileID:    chunk.ParsingProfileID,
		ChunkingProfileID:   chunk.ChunkingProfileID,
		Ordinal:             chunk.Ordinal,
		Text:                chunk.Text,
		ContentHash:         chunk.ContentHash,
		StartOffset:         chunk.StartOffset,
		EndOffset:           chunk.EndOffset,
		SourcePage:          chunk.SourcePage,
		SourceHeading:       chunk.SourceHeading,
		EmbeddingProfileIDs: cloneStrings(chunk.EmbeddingProfileIDs),
		Deleted:             chunk.Deletion.Deleted,
		DeletedAt:           cloneTime(chunk.Deletion.DeletedAt),
		DeleteReason:        chunk.Deletion.DeleteReason,
		SyncSequence:        chunk.SyncSequence,
		CreatedAt:           chunk.CreatedAt,
		UpdatedAt:           chunk.UpdatedAt,
	}
}

func ChunkDomainFromDocument(document po.ChunkDocument) domainknowledge.Chunk {
	return domainknowledge.Chunk{
		ID:                  document.ID,
		KnowledgeBaseID:     document.KnowledgeBaseID,
		DocumentID:          document.DocumentID,
		DocumentVersion:     document.DocumentVersion,
		ParsingProfileID:    document.ParsingProfileID,
		ChunkingProfileID:   document.ChunkingProfileID,
		Ordinal:             document.Ordinal,
		Text:                document.Text,
		ContentHash:         document.ContentHash,
		StartOffset:         document.StartOffset,
		EndOffset:           document.EndOffset,
		SourcePage:          document.SourcePage,
		SourceHeading:       document.SourceHeading,
		EmbeddingProfileIDs: cloneStrings(document.EmbeddingProfileIDs),
		Deletion: domainknowledge.Deletion{
			Deleted:      document.Deleted,
			DeletedAt:    cloneTime(document.DeletedAt),
			DeleteReason: document.DeleteReason,
		},
		SyncSequence: document.SyncSequence,
		CreatedAt:    document.CreatedAt,
		UpdatedAt:    document.UpdatedAt,
	}
}
