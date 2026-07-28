package subagent

type Scheduler interface {
	Submit(taskID string, task ScheduledTask) error
	Cancel(taskID string) bool
}
