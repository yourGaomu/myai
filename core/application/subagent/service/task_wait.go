package service

import (
	"context"
	"errors"
	"strings"
	"time"

	subagentcommand "myai/core/application/subagent/command"
	subagentresult "myai/core/application/subagent/result"
	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

const (
	defaultTaskWaitTimeout = 60 * time.Second
	minTaskWaitTimeout     = 100 * time.Millisecond
	maxTaskWaitTimeout     = 10 * time.Minute
)

// Wait blocks the caller's async operation until the child reaches a
// terminal state or the timeout expires. It consumes TaskEventSource events;
// it deliberately does not poll the task repository.
func (service *Service) Wait(ctx context.Context, command subagentcommand.WaitTask) (subagentresult.Wait, error) {
	if service == nil || service.Tasks == nil {
		return subagentresult.Wait{}, errors.New("subagent task repository is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	targetIDs, err := waitTargetIDs(command)
	if err != nil {
		return subagentresult.Wait{}, err
	}
	parentID := strings.TrimSpace(command.ParentSessionID)
	timeout := command.Timeout
	if timeout <= 0 {
		timeout = defaultTaskWaitTimeout
	}
	if timeout < minTaskWaitTimeout {
		timeout = minTaskWaitTimeout
	}
	if timeout > maxTaskWaitTimeout {
		timeout = maxTaskWaitTimeout
	}

	// Subscribe before reading state: a completion between the state read and
	// subscription must not depend on the bounded replay buffer retaining it.
	var events <-chan subagentport.TaskEvent
	if source, ok := service.Events.(subagentport.TaskEventSource); ok {
		var unsubscribe func()
		events, unsubscribe = source.SubscribeTaskEvents(parentID, 0, 32)
		defer unsubscribe()
	}
	tasks, err := service.loadWaitTasks(ctx, targetIDs, parentID)
	if err != nil {
		return subagentresult.Wait{}, err
	}
	if terminalTasks := terminalWaitTasks(tasks); len(terminalTasks) > 0 {
		return service.completedWaitResult(terminalTasks, 0)
	}
	waiter, err := service.markWaitingForChildren(command)
	if err != nil {
		return subagentresult.Wait{}, err
	}
	var wakeup <-chan struct{}
	if waiter != nil {
		defer service.restoreRunningAfterChildren(command.ParentTaskID, waiter)
		wakeup = waiter.wakeup
		if service.parkScheduler(waiter.runID) {
			defer service.unparkScheduler(waiter.runID)
		}
	}
	if events == nil {
		return subagentresult.Wait{}, errors.New("subagent task event source is not configured")
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return subagentresult.Wait{}, ctx.Err()
		case <-timer.C:
			current, err := service.loadWaitTasks(ctx, targetIDs, parentID)
			if err != nil {
				return subagentresult.Wait{}, err
			}
			terminal := terminalWaitTasks(current)
			if len(terminal) > 0 {
				return service.completedWaitResult(terminal, 0)
			}
			return newWaitResult(current, true, false, 0), nil
		case <-wakeup:
			current, err := service.loadWaitTasks(ctx, targetIDs, parentID)
			if err != nil {
				return subagentresult.Wait{}, err
			}
			return newWaitResult(current, false, true, 0), nil
		case event, open := <-events:
			if !open {
				return subagentresult.Wait{}, errors.New("subagent task event stream closed; reconnect and retry")
			}
			if !containsWaitTarget(targetIDs, event.Task.ID) {
				continue
			}
			current, err := service.loadWaitTasks(ctx, targetIDs, parentID)
			if err != nil {
				return subagentresult.Wait{}, err
			}
			if terminal := terminalWaitTasks(current); len(terminal) > 0 {
				return service.completedWaitResult(terminal, event.Sequence)
			}
		}
	}
}

func (service *Service) completedWaitResult(tasks []domainsubagent.Task, sequence uint64) (subagentresult.Wait, error) {
	// A terminal status can become visible before its completion callback.
	// Queue the same stable event before returning, so AgentLoop can consume it
	// at the wait boundary instead of generating a second answer later.
	for _, task := range tasks {
		if task.Status == domainsubagent.TaskStatusSucceeded || task.Status == domainsubagent.TaskStatusFailed {
			if err := service.ensureParentCompletionQueued(task); err != nil {
				return subagentresult.Wait{}, err
			}
		}
	}
	result := newWaitResult(tasks, false, false, sequence)
	result.ResultsInMailbox = service.ParentNotifier != nil
	return result, nil
}

func waitTargetIDs(command subagentcommand.WaitTask) ([]string, error) {
	ids := make([]string, 0, len(command.Targets)+1)
	seen := make(map[string]struct{}, len(command.Targets)+1)
	appendID := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		ids = append(ids, value)
	}
	for _, id := range command.Targets {
		appendID(id)
	}
	appendID(command.TaskID)
	if len(ids) == 0 {
		return nil, errors.New("subagent task id is required")
	}
	return ids, nil
}

func containsWaitTarget(targetIDs []string, taskID string) bool {
	for _, targetID := range targetIDs {
		if targetID == taskID {
			return true
		}
	}
	return false
}

func (service *Service) loadWaitTasks(ctx context.Context, targetIDs []string, parentID string) ([]domainsubagent.Task, error) {
	tasks := make([]domainsubagent.Task, 0, len(targetIDs))
	for _, taskID := range targetIDs {
		task, err := service.Tasks.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if err := validateWaitParent(task, parentID); err != nil {
			return nil, err
		}
		tasks = append(tasks, domainsubagent.CloneTask(task))
	}
	return tasks, nil
}

func terminalWaitTasks(tasks []domainsubagent.Task) []domainsubagent.Task {
	terminal := make([]domainsubagent.Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Terminal() {
			terminal = append(terminal, domainsubagent.CloneTask(task))
		}
	}
	return terminal
}

func newWaitResult(tasks []domainsubagent.Task, timedOut, wokenByMailbox bool, sequence uint64) subagentresult.Wait {
	result := subagentresult.Wait{TimedOut: timedOut, WokenByMailbox: wokenByMailbox, Sequence: sequence}
	if len(tasks) == 0 {
		return result
	}
	result.Tasks = make([]domainsubagent.Task, 0, len(tasks))
	for _, task := range tasks {
		cloned := domainsubagent.CloneTask(task)
		result.Tasks = append(result.Tasks, cloned)
		if result.Task.ID == "" {
			result.Task = cloned
		}
	}
	return result
}

func (service *Service) markWaitingForChildren(command subagentcommand.WaitTask) (*childWait, error) {
	parentTaskID := strings.TrimSpace(command.ParentTaskID)
	if parentTaskID == "" {
		return nil, nil
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	parent, err := service.Tasks.GetTask(context.Background(), parentTaskID)
	if err != nil {
		return nil, err
	}
	if parent.ChildSessionID != strings.TrimSpace(command.ParentSessionID) {
		return nil, subagentport.ErrNotFound
	}
	if parent.Status != domainsubagent.TaskStatusRunning && parent.Status != domainsubagent.TaskStatusWaitingSubagents {
		return nil, nil
	}
	expectedRunID := parent.CurrentRunID
	expectedUpdatedAt := parent.UpdatedAt
	changed := parent.Status == domainsubagent.TaskStatusRunning
	if changed {
		parent.Status = domainsubagent.TaskStatusWaitingSubagents
		parent.UpdatedAt = service.now()
		if err := service.saveTaskMutationDuringRun(context.Background(), parent, expectedRunID, expectedUpdatedAt); err != nil {
			return nil, err
		}
		if service.Runtime != nil {
			_ = service.Runtime.SetStatus(parent.ID, domainsubagent.AgentStatusWaiting)
		}
		service.publish(context.Background(), parent)
	}
	if service.waitingWakeups == nil {
		service.waitingWakeups = make(map[string]map[*childWait]struct{})
	}
	if service.waitingWakeups[parent.ID] == nil {
		service.waitingWakeups[parent.ID] = make(map[*childWait]struct{})
	}
	waiter := &childWait{runID: parent.CurrentRunID, wakeup: make(chan struct{})}
	service.waitingWakeups[parent.ID][waiter] = struct{}{}
	// A message may have arrived before registration. Delivering messages
	// already belong to the current model turn and must not wake its own wait.
	for _, message := range parent.Mailbox {
		if message.Status == "" || message.Status == domainsubagent.MessageStatusPending {
			close(waiter.wakeup)
			waiter.signaled = true
			break
		}
	}
	return waiter, nil
}

func (service *Service) restoreRunningAfterChildren(parentTaskID string, waiter *childWait) {
	if strings.TrimSpace(parentTaskID) == "" {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	parentTaskID = strings.TrimSpace(parentTaskID)
	waiters := service.waitingWakeups[parentTaskID]
	if _, registered := waiters[waiter]; !registered {
		return
	}
	delete(waiters, waiter)
	if len(waiters) == 0 {
		delete(service.waitingWakeups, parentTaskID)
	}
	for other := range waiters {
		if other.runID == waiter.runID {
			return
		}
	}
	parent, err := service.Tasks.GetTask(context.Background(), parentTaskID)
	if err != nil || parent.CurrentRunID != waiter.runID || parent.Status != domainsubagent.TaskStatusWaitingSubagents {
		return
	}
	parent.Status = domainsubagent.TaskStatusRunning
	expectedRunID := parent.CurrentRunID
	expectedUpdatedAt := parent.UpdatedAt
	parent.UpdatedAt = service.now()
	if err := service.saveTaskMutationDuringRun(context.Background(), parent, expectedRunID, expectedUpdatedAt); err == nil {
		if service.Runtime != nil {
			_ = service.Runtime.SetStatus(parent.ID, domainsubagent.AgentStatusRunning)
		}
		service.publish(context.Background(), parent)
	}
}

func (service *Service) parkScheduler(runID string) bool {
	if service == nil || service.Scheduler == nil || strings.TrimSpace(runID) == "" {
		return false
	}
	return service.Scheduler.Park(runID) == nil
}

func (service *Service) unparkScheduler(runID string) {
	if service == nil || service.Scheduler == nil || strings.TrimSpace(runID) == "" {
		return
	}
	_ = service.Scheduler.Unpark(runID)
}

func validateWaitParent(task domainsubagent.Task, parentID string) error {
	if parentID != "" && task.ParentSessionID != parentID {
		return subagentport.ErrNotFound
	}
	return nil
}
