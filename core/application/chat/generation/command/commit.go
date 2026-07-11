package command

import (
	modelport "myai/core/port/model"
	"myai/core/session"
)

type Commit struct {
	Session     *session.Session
	LatestInput string
	Result      modelport.ChatResult
	CapturePlan bool
}
