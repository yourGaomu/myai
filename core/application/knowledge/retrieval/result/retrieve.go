package result

import domainknowledge "myai/core/domain/knowledge"

type Diagnostics struct {
	LocalVectorHits  int
	LocalKeywordHits int
	RemoteVectorHits int
	RemoteFallback   bool
	CacheFillCount   int
	LocalQuality     domainknowledge.LocalQualityDecision
	Warnings         []string
}

type Retrieve struct {
	Hits        []domainknowledge.RetrievalHit
	Diagnostics Diagnostics
}
