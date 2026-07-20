package mapper

import (
	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func SyncChangeDocumentFromDomain(change domainknowledge.SyncChange) po.SyncChangeDocument {
	return po.SyncChangeDocument{
		ID:              change.ID,
		Sequence:        change.Sequence,
		KnowledgeBaseID: change.KnowledgeBaseID,
		EntityType:      change.EntityType,
		EntityID:        change.EntityID,
		Operation:       string(change.Operation),
		EntityVersion:   change.EntityVersion,
		Deleted:         change.Deleted,
		OccurredAt:      change.OccurredAt,
	}
}

func SyncChangeDomainFromDocument(document po.SyncChangeDocument) domainknowledge.SyncChange {
	return domainknowledge.SyncChange{
		ID:              document.ID,
		Sequence:        document.Sequence,
		KnowledgeBaseID: document.KnowledgeBaseID,
		EntityType:      document.EntityType,
		EntityID:        document.EntityID,
		Operation:       domainknowledge.SyncOperation(document.Operation),
		EntityVersion:   document.EntityVersion,
		Deleted:         document.Deleted,
		OccurredAt:      document.OccurredAt,
	}
}
