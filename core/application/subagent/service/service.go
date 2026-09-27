package service

import (
	"sync"
	"time"

	modelport "myai/core/port/model"
	subagentport "myai/core/port/subagent"
	workspaceport "myai/core/port/workspace"
)

type Service struct {
	Definitions          subagentport.DefinitionRepository
	Registry             subagentport.DefinitionRegistry
	Tasks                subagentport.TaskRepository
	Runs                 subagentport.RunRepository
	TaskRuns             subagentport.TaskRunRepository
	ExecutionLease       subagentport.AgentExecutionLeaseStore
	ExecutionOwnerID     string
	ExecutionLeaseTTL    time.Duration
	MessageOwnerID       string
	AgentMessages        subagentport.AgentMessageRepository
	AgentPaths           subagentport.AgentPathRegistry
	Runtime              subagentport.AgentRuntimeManager
	Models               modelport.Registry
	Scheduler            subagentport.Scheduler
	Sessions             subagentport.ChildSessionFactory
	Runner               subagentport.AgentRunner
	ParentContinuation   subagentport.ParentContinuation
	ParentNotifier       subagentport.ParentCompletionNotifier
	IDs                  subagentport.IDGenerator
	Events               subagentport.EventPublisher
	Workspaces           workspaceport.Manager
	DefaultWorkspaceRoot string
	Now                  func() time.Time
	OnError              func(error)

	mu             sync.Mutex
	admissionMu    sync.Mutex
	resumeClaims   map[string]struct{}
	parentNotified map[string]struct{}
	activeRuns     map[string]*activeExecution
	waitingWakeups map[string]map[*childWait]struct{}
}

type activeExecution struct {
	runID string
	done  chan struct{}
}

type childWait struct {
	runID    string
	wakeup   chan struct{}
	signaled bool
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
