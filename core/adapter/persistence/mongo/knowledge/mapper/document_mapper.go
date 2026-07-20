package mapper

import (
	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func KnowledgeDocumentFromDomain(document domainknowledge.Document) po.KnowledgeDocument {
	return po.KnowledgeDocument{
		ID:              document.ID,
		KnowledgeBaseID: document.KnowledgeBaseID,
		FileName:        document.FileName,
		ContentType:     document.ContentType,
		ObjectKey:       document.ObjectKey,
		ContentHash:     document.ContentHash,
		Version:         document.Version,
		Status:          string(document.Status),
		FailureReason:   document.FailureReason,
		Deleted:         document.Deletion.Deleted,
		DeletedAt:       cloneTime(document.Deletion.DeletedAt),
		DeleteReason:    document.Deletion.DeleteReason,
		SyncSequence:    document.SyncSequence,
		CreatedAt:       document.CreatedAt,
		UpdatedAt:       document.UpdatedAt,
	}
}

func KnowledgeDocumentDomainFromDocument(document po.KnowledgeDocument) domainknowledge.Document {
	return domainknowledge.Document{
		ID:              document.ID,
		KnowledgeBaseID: document.KnowledgeBaseID,
		FileName:        document.FileName,
		ContentType:     document.ContentType,
		ObjectKey:       document.ObjectKey,
		ContentHash:     document.ContentHash,
		Version:         document.Version,
		Status:          domainknowledge.DocumentStatus(document.Status),
		FailureReason:   document.FailureReason,
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
