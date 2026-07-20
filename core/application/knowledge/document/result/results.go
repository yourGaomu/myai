package result

import domainknowledge "myai/core/domain/knowledge"

type Ingest struct {
	Document domainknowledge.Document
	Job      domainknowledge.IndexingJob
}

type Retry struct {
	Job domainknowledge.IndexingJob
}
