package memory

import domainmemory "myai/core/domain/memory"

type ListFilter struct {
	Text           string
	Tags           []string
	Kinds          []domainmemory.Kind
	Statuses       []domainmemory.Status
	ScopeTypes     []domainmemory.ScopeType
	ScopeKey       string
	IncludeDeleted bool
	Limit          int
}

type CandidateFilter struct {
	Statuses []domainmemory.CandidateStatus
	Limit    int
}
