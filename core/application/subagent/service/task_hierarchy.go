package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	subagentcommand "myai/core/application/subagent/command"
	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

const (
	maxSubagentDepth   = 4
	maxSubagentFanout  = 8
	maxParentChainWalk = 64
)

// validateTaskParent establishes the runtime parent relationship before a
// nested task is persisted. ParentSessionID must be the parent task's child
// session, otherwise a task could attach itself to another agent's tree.
func (service *Service) validateTaskParent(ctx context.Context, command subagentcommand.StartTask) (domainsubagent.Task, error) {
	parentID := strings.TrimSpace(command.ParentTaskID)
	if parentID == "" {
		return domainsubagent.Task{}, nil
	}
	parent, err := service.Tasks.GetTask(ctx, parentID)
	if err != nil {
		return domainsubagent.Task{}, fmt.Errorf("load parent subagent task: %w", err)
	}
	if strings.TrimSpace(command.ParentSessionID) == "" || parent.ChildSessionID != strings.TrimSpace(command.ParentSessionID) {
		return domainsubagent.Task{}, subagentport.ErrNotFound
	}
	if parent.Terminal() {
		return domainsubagent.Task{}, errors.New("cannot spawn a child from a terminal subagent task")
	}
	children, err := service.Tasks.ListTasks(ctx, parent.ChildSessionID, maxSubagentFanout+1)
	if err != nil {
		return domainsubagent.Task{}, fmt.Errorf("count child subagent tasks: %w", err)
	}
	if len(children) >= maxSubagentFanout {
		return domainsubagent.Task{}, fmt.Errorf("subagent fan-out limit reached: maximum %d children", maxSubagentFanout)
	}
	depth := 1
	current := parent
	seen := map[string]struct{}{current.ID: {}}
	for strings.TrimSpace(current.ParentTaskID) != "" {
		if depth >= maxParentChainWalk {
			return domainsubagent.Task{}, errors.New("subagent parent chain is too deep or cyclic")
		}
		ancestor, loadErr := service.Tasks.GetTask(ctx, current.ParentTaskID)
		if loadErr != nil {
			return domainsubagent.Task{}, fmt.Errorf("load ancestor subagent task: %w", loadErr)
		}
		if _, exists := seen[ancestor.ID]; exists {
			return domainsubagent.Task{}, errors.New("subagent parent chain is cyclic")
		}
		seen[ancestor.ID] = struct{}{}
		depth++
		current = ancestor
	}
	if depth >= maxSubagentDepth {
		return domainsubagent.Task{}, fmt.Errorf("subagent depth limit reached: maximum %d levels", maxSubagentDepth)
	}
	return parent, nil
}

func nestedAgentPath(parent domainsubagent.Task, taskName string, fallbackID string) string {
	name := strings.TrimSpace(taskName)
	if name == "" {
		name = fallbackID
	}
	prefix := strings.Trim(strings.TrimSpace(parent.AgentPath), "/")
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}
