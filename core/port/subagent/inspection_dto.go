package subagent

type TaskEventFilter struct {
	ParentSessionID string
	TaskID          string
	RunID           string
	AfterSequence   uint64
	ThroughSequence uint64
	Limit           int
}
