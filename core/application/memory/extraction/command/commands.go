package command

type EnqueueRun struct {
	AgentRunID string
}

type Process struct {
	JobID string
}

type Recover struct {
	Limit int
}
