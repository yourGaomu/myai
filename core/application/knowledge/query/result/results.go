package result

import domainknowledge "myai/core/domain/knowledge"

type Documents struct {
	Documents []domainknowledge.Document
	Jobs      []domainknowledge.IndexingJob
}

type IndexProfiles struct {
	Profiles []domainknowledge.IndexProfile
}
