package result

import domainmemory "myai/core/domain/memory"

type List struct {
	Memories []domainmemory.Memory
}

type Detail struct {
	Memory domainmemory.Memory
}

type Candidate struct {
	MemoryCandidate domainmemory.Candidate
}

type Candidates struct {
	Items []domainmemory.Candidate
}
