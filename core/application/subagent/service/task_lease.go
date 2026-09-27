package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

const defaultExecutionLeaseTTL = 30 * time.Second

func (service *Service) leaseTTL() time.Duration {
	if service.ExecutionLeaseTTL > 0 {
		return service.ExecutionLeaseTTL
	}
	return defaultExecutionLeaseTTL
}

func (service *Service) leaseRenewTimeout() time.Duration {
	timeout := service.leaseTTL() / 3
	if timeout <= 0 {
		return time.Second
	}
	if timeout > 5*time.Second {
		return 5 * time.Second
	}
	return timeout
}

func (service *Service) acquireExecutionLease(ctx context.Context, taskID, runID string) error {
	if service.ExecutionLease == nil {
		return nil
	}
	if strings.TrimSpace(service.ExecutionOwnerID) == "" {
		return errors.New("subagent execution lease owner id is not configured")
	}
	return service.ExecutionLease.Acquire(ctx, taskID, runID, service.ExecutionOwnerID, service.leaseTTL())
}

func (service *Service) renewExecutionLease(ctx context.Context, taskID, runID string) error {
	if service.ExecutionLease == nil {
		return nil
	}
	return service.ExecutionLease.Renew(ctx, taskID, runID, service.ExecutionOwnerID, service.leaseTTL())
}

func (service *Service) releaseExecutionLease(taskID, runID string) {
	if service.ExecutionLease == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := service.ExecutionLease.Release(ctx, taskID, runID, service.ExecutionOwnerID); err != nil && !errors.Is(err, subagentport.ErrExecutionLeaseNotAcquired) {
		service.reportError(fmt.Errorf("release subagent execution lease %s/%s: %w", taskID, runID, err))
	}
}

// watchExecutionLease cancels local model/tool work when ownership is lost.
// The caller waits for the returned channel before releasing the lease so a
// delayed renewal cannot extend ownership after cleanup.
func (service *Service) watchExecutionLease(ctx context.Context, taskID, runID string, cancel context.CancelFunc, lost *atomic.Bool) <-chan struct{} {
	done := make(chan struct{})
	if service.ExecutionLease == nil {
		close(done)
		return done
	}
	interval := service.leaseTTL() / 3
	if interval <= 0 {
		interval = time.Second
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				renewCtx, renewCancel := context.WithTimeout(ctx, service.leaseRenewTimeout())
				err := service.renewExecutionLease(renewCtx, taskID, runID)
				renewCancel()
				if err != nil {
					if ctx.Err() == nil {
						lost.Store(true)
						service.reportError(fmt.Errorf("subagent execution lease lost %s/%s: %w", taskID, runID, err))
						cancel()
					}
					return
				}
				// Cancellation is a task state transition and may be initiated by
				// another application instance. The local scheduler cannot receive
				// that signal directly, so the lease owner observes the durable task
				// record and cancels the model/tool context cooperatively.
				if service.Tasks != nil {
					current, loadErr := service.Tasks.GetTask(ctx, taskID)
					if loadErr == nil && (current.CurrentRunID != runID || current.Status == domainsubagent.TaskStatusCanceled) {
						cancel()
						return
					}
				}
			}
		}
	}()
	return done
}
