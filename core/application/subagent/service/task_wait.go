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

	taskID := strings.TrimSpace(command.TaskID)
	if taskID == "" {
		return subagentresult.Wait{}, errors.New("subagent task id is required")
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

	if task, err := service.Tasks.GetTask(ctx, taskID); err != nil {
		return subagentresult.Wait{}, err
	} else if err := validateWaitParent(task, parentID); err != nil {
		return subagentresult.Wait{}, err
	} else if task.Terminal() {
		return subagentresult.Wait{Task: domainsubagent.CloneTask(task)}, nil
	}
	waiter, err := service.markWaitingForChildren(command)
	if err != nil {
		return subagentresult.Wait{}, err
	}
	var wakeup <-chan struct{}
	if waiter != nil {
		defer service.restoreRunningAfterChildren(command.ParentTaskID, waiter)
		wakeup = waiter.wakeup
	}
	source, ok := service.Events.(subagentport.TaskEventSource)
	if !ok {
		return subagentresult.Wait{}, errors.New("subagent task event source is not configured")
	}

	// Subscribe after the initial read; replay from sequence zero covers a
	// completion that races with this subscription.
	events, unsubscribe := source.SubscribeTaskEvents(parentID, 0, 32)
	defer unsubscribe()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return subagentresult.Wait{}, ctx.Err()
		case <-timer.C:
			task, err := service.Tasks.GetTask(context.Background(), taskID)
			if err != nil {
				return subagentresult.Wait{}, err
			}
			return subagentresult.Wait{Task: task, TimedOut: !task.Terminal()}, nil
		case <-wakeup:
			task, err := service.Tasks.GetTask(context.Background(), taskID)
			if err != nil {
				return subagentresult.Wait{}, err
			}
			return subagentresult.Wait{Task: task, WokenByMailbox: true}, nil
		case event, open := <-events:
			if !open {
				return subagentresult.Wait{}, errors.New("subagent task event stream closed; reconnect and retry")
			}
			if event.Task.ID != taskID {
				continue
			}
			current, err := service.Tasks.GetTask(context.Background(), taskID)
			if err != nil {
				return subagentresult.Wait{}, err
			}
			if err := validateWaitParent(current, parentID); err != nil {
				return subagentresult.Wait{}, err
			}
			if current.Terminal() {
				return subagentresult.Wait{Task: current, Sequence: event.Sequence}, nil
			}
		}
	}
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
	changed := parent.Status == domainsubagent.TaskStatusRunning
	if changed {
		parent.Status = domainsubagent.TaskStatusWaitingSubagents
		parent.UpdatedAt = service.now()
		if err := service.Tasks.SaveTask(context.Background(), parent); err != nil {
			return nil, err
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
	parent.UpdatedAt = service.now()
	if err := service.Tasks.SaveTask(context.Background(), parent); err == nil {
		service.publish(context.Background(), parent)
	}
}

func validateWaitParent(task domainsubagent.Task, parentID string) error {
	if parentID != "" && task.ParentSessionID != parentID {
		return subagentport.ErrNotFound
	}
	return nil
}
