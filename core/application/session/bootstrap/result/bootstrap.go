package result

import "myai/core/session"

type Action string

const (
	ActionLoaded  Action = "loaded"
	ActionCreated Action = "created"
	ActionReused  Action = "reused"
)

type Bootstrap struct {
	Session *session.Session
	Action  Action
}
