package mapper

import (
	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func KnowledgeBaseDocumentFromDomain(base domainknowledge.KnowledgeBase) po.KnowledgeBaseDocument {
	return po.KnowledgeBaseDocument{
		ID:                    base.ID,
		CategoryID:            base.CategoryID,
		Name:                  base.Name,
		Description:           base.Description,
		RAGEnabled:            base.RAGEnabled,
		ActiveIndexProfileID:  base.ActiveIndexProfileID,
		PendingIndexProfileID: base.PendingIndexProfileID,
		Deleted:               base.Deletion.Deleted,
		DeletedAt:             cloneTime(base.Deletion.DeletedAt),
		DeleteReason:          base.Deletion.DeleteReason,
		SyncSequence:          base.SyncSequence,
		CreatedAt:             base.CreatedAt,
		UpdatedAt:             base.UpdatedAt,
	}
}

func KnowledgeBaseDomainFromDocument(document po.KnowledgeBaseDocument) domainknowledge.KnowledgeBase {
	return domainknowledge.KnowledgeBase{
		ID:                    document.ID,
		CategoryID:            document.CategoryID,
		Name:                  document.Name,
		Description:           document.Description,
		RAGEnabled:            document.RAGEnabled,
		ActiveIndexProfileID:  document.ActiveIndexProfileID,
		PendingIndexProfileID: document.PendingIndexProfileID,
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
