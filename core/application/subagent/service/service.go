package service

import (
	"sync"
	"time"

	subagentport "myai/core/port/subagent"
	workspaceport "myai/core/port/workspace"
)

type Service struct {
	Definitions          subagentport.DefinitionRepository
	Registry             subagentport.DefinitionRegistry
	Tasks                subagentport.TaskRepository
	Runs                 subagentport.RunRepository
	TaskRuns             subagentport.TaskRunRepository
	Scheduler            subagentport.Scheduler
	Sessions             subagentport.ChildSessionFactory
	Runner               subagentport.AgentRunner
	ParentContinuation   subagentport.ParentContinuation
	IDs                  subagentport.IDGenerator
	Events               subagentport.EventPublisher
	Workspaces           workspaceport.Manager
	DefaultWorkspaceRoot string
	Now                  func() time.Time
	OnError              func(error)

	mu           sync.Mutex
	admissionMu  sync.Mutex
	resumeClaims map[string]struct{}
	activeRuns   map[string]chan struct{}
}

func (service *Service) reportError(err error) {
	if service != nil && err != nil && service.OnError != nil {
		service.OnError(err)
	}
}

func (service *Service) now() time.Time {
	if service != nil && service.Now != nil {
		return service.Now()
	}
	return time.Now().UTC()
}
