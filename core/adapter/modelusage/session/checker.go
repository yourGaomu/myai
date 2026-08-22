package session

import (
	"context"
	"strings"

	modelappport "myai/core/application/model/port"
	repository "myai/core/port/repository"
)

type SessionReader interface {
	ListSessions(ctx context.Context) ([]repository.SessionRecord, error)
}

type Checker struct {
	Sessions SessionReader
}

var _ modelappport.UsageChecker = Checker{}

func (c Checker) IsModelInUse(ctx context.Context, modelID string) (bool, error) {
	if c.Sessions == nil {
		return false, nil
	}
	modelID = strings.TrimSpace(modelID)
	sessions, err := c.Sessions.ListSessions(ctx)
	if err != nil {
		return false, err
	}
	for _, current := range sessions {
		if !current.Deleted && strings.TrimSpace(current.Model) == modelID {
			return true, nil
		}
	}
	return false, nil
}
