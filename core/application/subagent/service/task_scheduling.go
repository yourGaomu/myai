package service

import (
	"context"
	"errors"

	subagentresult "myai/core/application/subagent/result"
	domainsubagent "myai/core/domain/subagent"
)

// Each Run owns a scheduler entry. The previous job may still be draining
// after execute has released its service-level ownership.
func (service *Service) scheduleRun(task domainsubagent.Task, run domainsubagent.Run, fallbackModelID string) (subagentresult.Task, error) {
	err := service.Scheduler.Submit(run.ID, func(ctx context.Context) {
		service.execute(ctx, task.ID, run.ID, fallbackModelID)
	})
	if err == nil {
		return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	defer service.completeActiveRunLocked(task.ID, run.ID)
	current, loadErr := service.Tasks.GetTask(context.Background(), task.ID)
	if loadErr != nil {
		return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, errors.Join(err, loadErr)
	}
	if current.CurrentRunID != run.ID || current.Terminal() {
		return subagentresult.Task{Value: domainsubagent.CloneTask(current)}, err
	}
	now := service.now()
	message := "schedule subagent task: " + err.Error()
	if transitionErr := current.MarkFailed(message, now); transitionErr != nil {
		return subagentresult.Task{Value: domainsubagent.CloneTask(current)}, errors.Join(err, transitionErr)
	}
	run.Status = domainsubagent.RunStatusFailed
	run.ErrorMessage = message
	run.CompletedAt = timePtr(now)
	if saveErr := service.saveTaskAndRun(context.Background(), current, run); saveErr != nil {
		return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, errors.Join(err, saveErr)
	}
	service.publish(context.Background(), current)
	return subagentresult.Task{Value: domainsubagent.CloneTask(current)}, err
}
