package result

import domainmemory "myai/core/domain/memory"

type Job struct {
	ExtractionJob domainmemory.ExtractionJob
}

type Jobs struct {
	Items []domainmemory.ExtractionJob
}
