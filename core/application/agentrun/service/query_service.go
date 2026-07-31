package service

import (
	"context"
	"errors"
	"strings"

	agentrunapi "myai/core/application/agentrun/api"
	agentrunquery "myai/core/application/agentrun/query"
	agentrunresult "myai/core/application/agentrun/result"
	domainagentrun "myai/core/domain/agentrun"
	agentrunport "myai/core/port/agentrun"
)

type QueryService struct {
	Repository agentrunport.Repository
}

var _ agentrunapi.QueryService = QueryService{}

func (s QueryService) ListSessionRuns(ctx context.Context, query agentrunquery.ListSessionRuns) ([]agentrunresult.Snapshot, error) {
	if s.Repository == nil {
		return nil, errors.New("agent run repository is nil")
	}
	sessionID := strings.TrimSpace(query.SessionID)
	if sessionID == "" {
		return nil, errors.New("agent run session id is empty")
	}
	runs, err := s.Repository.ListRuns(ctx, sessionID, query.Limit)
	if err != nil {
		return nil, err
	}
	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		runIDs = append(runIDs, run.ID)
	}
	events, err := s.Repository.ListEvents(ctx, runIDs)
	if err != nil {
		return nil, err
	}
	byRun := make(map[string][]domainagentrun.Event, len(runs))
	for _, event := range events {
		byRun[event.RunID] = append(byRun[event.RunID], event)
	}
	result := make([]agentrunresult.Snapshot, 0, len(runs))
	for _, run := range runs {
		result = append(result, agentrunresult.Snapshot{Run: run, Events: byRun[run.ID]})
	}
	return result, nil
}
