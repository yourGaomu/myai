package command

import domainmemory "myai/core/domain/memory"

type EnqueueRun struct {
	AgentRunID string
}

type Process struct {
	JobID string
}

type Recover struct {
	Limit int
}

type ListJobs struct {
	Statuses []domainmemory.JobStatus
	Limit    int
}

type Retry struct {
	JobID string
}
