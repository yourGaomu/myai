package command

import (
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
)

type List struct {
	Filter memoryport.ListFilter
}

type Get struct {
	MemoryID string
}

type Create struct {
	Title      string
	Kind       domainmemory.Kind
	Scope      domainmemory.Scope
	Tags       []string
	Content    domainmemory.Content
	Confidence float64
}

type Update struct {
	MemoryID   string
	Title      string
	Kind       domainmemory.Kind
	Scope      domainmemory.Scope
	Tags       []string
	Content    domainmemory.Content
	Confidence float64
}

type Delete struct {
	MemoryID string
	Reason   string
}

type Restore struct {
	MemoryID string
}

type CreateCandidate struct {
	Title      string
	Kind       domainmemory.Kind
	Scope      domainmemory.Scope
	Tags       []string
	Content    domainmemory.Content
	Confidence float64
	Sources    []domainmemory.SourceRef
}

type ListCandidates struct {
	Filter memoryport.CandidateFilter
}

type ApproveCandidate struct {
	CandidateID  string
	MemoryID     string
	HumanApprove bool
}

type RejectCandidate struct {
	CandidateID string
	Note        string
}

type RecordUse struct {
	MemoryID string
}
