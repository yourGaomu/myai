package command

import "myai/core/session"

type Prepare struct {
	Session *session.Session
	Input   string
}
