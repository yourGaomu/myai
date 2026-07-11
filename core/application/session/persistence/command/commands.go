package command

import (
	"time"

	repository "myai/core/port/repository"
	"myai/core/session"
)

type BuildRecord struct {
	SessionID    string
	Model        string
	Title        string
	DefaultModel string
	Current      *session.Session
	Existing     repository.SessionRecord
	HasExisting  bool
	Now          time.Time
}

type PrepareRecord struct {
	Record       repository.SessionRecord
	Existing     repository.SessionRecord
	HasExisting  bool
	DefaultModel string
	Now          time.Time
}
