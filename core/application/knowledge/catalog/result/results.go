package result

import domainknowledge "myai/core/domain/knowledge"

type Catalog struct {
	Categories     []domainknowledge.KnowledgeCategory
	KnowledgeBases []domainknowledge.KnowledgeBase
}

type Category struct {
	Category domainknowledge.KnowledgeCategory
}

type KnowledgeBase struct {
	KnowledgeBase domainknowledge.KnowledgeBase
}
