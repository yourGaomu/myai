package mapper

import (
	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func KnowledgeCategoryDocumentFromDomain(category domainknowledge.KnowledgeCategory) po.KnowledgeCategoryDocument {
	return po.KnowledgeCategoryDocument{
		ID:           category.ID,
		Name:         category.Name,
		ParentID:     category.ParentID,
		AncestorIDs:  cloneStrings(category.AncestorIDs),
		SortOrder:    category.SortOrder,
		Deleted:      category.Deletion.Deleted,
		DeletedAt:    cloneTime(category.Deletion.DeletedAt),
		DeleteReason: category.Deletion.DeleteReason,
		CreatedAt:    category.CreatedAt,
		UpdatedAt:    category.UpdatedAt,
	}
}

func KnowledgeCategoryDomainFromDocument(document po.KnowledgeCategoryDocument) domainknowledge.KnowledgeCategory {
	return domainknowledge.KnowledgeCategory{
		ID:          document.ID,
		Name:        document.Name,
		ParentID:    document.ParentID,
		AncestorIDs: cloneStrings(document.AncestorIDs),
		SortOrder:   document.SortOrder,
		Deletion: domainknowledge.Deletion{
			Deleted:      document.Deleted,
			DeletedAt:    cloneTime(document.DeletedAt),
			DeleteReason: document.DeleteReason,
		},
		CreatedAt: document.CreatedAt,
		UpdatedAt: document.UpdatedAt,
	}
}
