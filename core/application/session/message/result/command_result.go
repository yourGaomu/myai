package result

import "myai/core/session"

type Command struct {
	Session *session.Session
	Input   string
}
