package agent

import (
	"context"

	subagentcommand "myai/core/application/subagent/command"
	subagentresult "myai/core/application/subagent/result"
	domainsubagent "myai/core/domain/subagent"
)

type SubagentFacade interface {
	CreateDefinition(ctx context.Context, command subagentcommand.CreateDefinition) (subagentresult.Definition, error)
	UpdateDefinition(ctx context.Context, command subagentcommand.UpdateDefinition) (subagentresult.Definition, error)
	DeleteDefinition(ctx context.Context, command subagentcommand.DeleteDefinition) error
	ListDefinitions(ctx context.Context) (subagentresult.Definitions, error)
	Check(ctx context.Context, command subagentcommand.CheckTask) (subagentresult.Task, error)
	List(ctx context.Context, command subagentcommand.ListTasks) (subagentresult.Tasks, error)
	Cancel(ctx context.Context, command subagentcommand.CancelTask) (subagentresult.Task, error)
	Resume(ctx context.Context, command subagentcommand.ResumeTask) (subagentresult.Resume, error)
	ApplyChanges(ctx context.Context, command subagentcommand.ApplyTaskChanges) (subagentresult.Task, error)
	DiscardChanges(ctx context.Context, command subagentcommand.DiscardTaskChanges) (subagentresult.Task, error)
}

type SubagentEventSource interface {
	Subscribe(buffer int) (<-chan domainsubagent.Task, func())
}
