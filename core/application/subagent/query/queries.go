package query

type InspectTask struct {
	ParentSessionID string
	TaskID          string
}

type TaskEvents struct {
	ParentSessionID string
	TaskID          string
	RunID           string
	AfterSequence   uint64
	ThroughSequence uint64
	Limit           int
}
