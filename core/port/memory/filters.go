package memory

import domainmemory "myai/core/domain/memory"

const (
	DefaultListLimit = 50
	MaxListLimit     = 200
)

// NormalizeListLimit keeps explicit limits up to the retrieval pool size and
// clamps oversized requests instead of silently falling back to the default.
func NormalizeListLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}

type ListFilter struct {
	Text string
	// SearchTerms matches any lexical term before the candidate limit is applied.
	// Text retains its existing literal phrase semantics for catalog callers.
	SearchTerms    []string
	OrderByContent bool
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
