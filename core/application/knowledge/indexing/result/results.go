package result

import domainknowledge "myai/core/domain/knowledge"

type Submit struct {
	Job domainknowledge.IndexingJob
}

type Run struct {
	Job       domainknowledge.IndexingJob
	Document  domainknowledge.Document
	Processed *domainknowledge.ProcessingSummary
}
