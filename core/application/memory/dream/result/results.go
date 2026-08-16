package result

import domainmemory "myai/core/domain/memory"

type Run struct {
	DreamRun domainmemory.DreamRun
}

type List struct {
	Runs []domainmemory.DreamRun
}
