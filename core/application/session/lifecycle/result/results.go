package result

import "myai/core/session"

type Action string

const (
	ActionNew     Action = "new"
	ActionLoad    Action = "load"
	ActionDelete  Action = "delete"
	ActionRestore Action = "restore"
	ActionClear   Action = "clear"
)

type Lifecycle struct {
	SessionID string
	Current   *session.Session
	Action    Action
}

type DeleteSession struct {
	SessionID      string
	DeletedCurrent bool
}
