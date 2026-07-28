package local

import (
	"context"
	"errors"
	"log"
	"sync"

	subagentport "myai/core/port/subagent"
)

type scheduledJob struct {
	id     string
	ctx    context.Context
	cancel context.CancelFunc
	run    subagentport.ScheduledTask
}

type Scheduler struct {
	jobs   chan scheduledJob
	mu     sync.Mutex
	active map[string]context.CancelFunc
	closed bool
	wg     sync.WaitGroup
}

var _ subagentport.Scheduler = (*Scheduler)(nil)

func NewScheduler(workers int, queueSize int) *Scheduler {
	if workers < 1 {
		workers = 2
	}
	if queueSize < 1 {
		queueSize = 32
	}
	scheduler := &Scheduler{jobs: make(chan scheduledJob, queueSize), active: make(map[string]context.CancelFunc)}
	for index := 0; index < workers; index++ {
		scheduler.wg.Add(1)
		go scheduler.worker()
	}
	return scheduler
}

func (scheduler *Scheduler) Submit(taskID string, task subagentport.ScheduledTask) error {
	if scheduler == nil {
		return errors.New("subagent scheduler is nil")
	}
	if taskID == "" || task == nil {
		return errors.New("subagent scheduled task is invalid")
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.closed {
		return errors.New("subagent scheduler is closed")
	}
	if _, exists := scheduler.active[taskID]; exists {
		return errors.New("subagent task is already scheduled")
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := scheduledJob{id: taskID, ctx: ctx, cancel: cancel, run: task}
	select {
	case scheduler.jobs <- job:
		scheduler.active[taskID] = cancel
		return nil
	default:
		cancel()
		return errors.New("subagent task queue is full")
	}
}

func (scheduler *Scheduler) Cancel(taskID string) bool {
	if scheduler == nil {
		return false
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	cancel := scheduler.active[taskID]
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (scheduler *Scheduler) Close() {
	if scheduler == nil {
		return
	}
	scheduler.mu.Lock()
	if scheduler.closed {
		scheduler.mu.Unlock()
		return
	}
	scheduler.closed = true
	for _, cancel := range scheduler.active {
		cancel()
	}
	close(scheduler.jobs)
	scheduler.mu.Unlock()
	scheduler.wg.Wait()
}

func (scheduler *Scheduler) worker() {
	defer scheduler.wg.Done()
	for job := range scheduler.jobs {
		scheduler.runJob(job)
	}
}

func (scheduler *Scheduler) runJob(job scheduledJob) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("subagent scheduler recovered task %s panic: %v", job.id, recovered)
		}
		job.cancel()
		scheduler.mu.Lock()
		delete(scheduler.active, job.id)
		scheduler.mu.Unlock()
	}()
	job.run(job.ctx)
}
