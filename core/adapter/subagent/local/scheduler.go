package local

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	subagentport "myai/core/port/subagent"
)

type scheduledJob struct {
	id     string
	ctx    context.Context
	cancel context.CancelFunc
	run    subagentport.ScheduledTask
}

type activeJob struct {
	cancel    context.CancelFunc
	parkCount int
	unpark    chan struct{}
}

const defaultCloseTimeout = 30 * time.Second

type Scheduler struct {
	jobs     chan scheduledJob
	closedCh chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	active   map[string]*activeJob
	closed   bool
	wg       sync.WaitGroup
}

var _ subagentport.Scheduler = (*Scheduler)(nil)

func NewScheduler(workers int, queueSize int) *Scheduler {
	if workers < 1 {
		workers = 2
	}
	if queueSize < 1 {
		queueSize = 32
	}
	scheduler := &Scheduler{
		jobs:     make(chan scheduledJob, queueSize),
		closedCh: make(chan struct{}),
		done:     make(chan struct{}),
		active:   make(map[string]*activeJob),
	}
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
		return subagentport.ErrSchedulerClosed
	}
	if _, exists := scheduler.active[taskID]; exists {
		return errors.New("subagent task is already scheduled")
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := scheduledJob{id: taskID, ctx: ctx, cancel: cancel, run: task}
	select {
	case scheduler.jobs <- job:
		scheduler.active[taskID] = &activeJob{cancel: cancel}
		return nil
	default:
		cancel()
		return subagentport.ErrSchedulerQueueFull
	}
}

func (scheduler *Scheduler) Cancel(taskID string) bool {
	if scheduler == nil {
		return false
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	job := scheduler.active[taskID]
	if job == nil {
		return false
	}
	job.cancel()
	return true
}

func (scheduler *Scheduler) Park(taskID string) error {
	if scheduler == nil || taskID == "" {
		return errors.New("subagent scheduler park is invalid")
	}
	scheduler.mu.Lock()
	if scheduler.closed {
		scheduler.mu.Unlock()
		return subagentport.ErrSchedulerClosed
	}
	job := scheduler.active[taskID]
	if job == nil {
		scheduler.mu.Unlock()
		return errors.New("subagent task is not scheduled")
	}
	job.parkCount++
	if job.unpark == nil {
		job.unpark = make(chan struct{})
	}
	unpark := job.unpark
	scheduler.wg.Add(1)
	scheduler.mu.Unlock()
	go scheduler.substituteWorker(unpark)
	return nil
}

func (scheduler *Scheduler) Unpark(taskID string) error {
	if scheduler == nil || taskID == "" {
		return nil
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	job := scheduler.active[taskID]
	if job == nil || job.parkCount == 0 {
		return nil
	}
	job.parkCount--
	if job.parkCount == 0 && job.unpark != nil {
		close(job.unpark)
		job.unpark = nil
	}
	return nil
}

// Close stops admission, cancels accepted tasks and waits up to 30 seconds.
// Call CloseContext to handle cancellation or an incomplete shutdown explicitly.
func (scheduler *Scheduler) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
	defer cancel()
	if err := scheduler.CloseContext(ctx); err != nil {
		log.Printf("subagent scheduler close incomplete: %v", err)
	}
}

// CloseContext stops admission and cancels both running and queued tasks before
// waiting for every worker, including substitutes created by Park. Queued task
// callbacks still run with a canceled context so their owners can clean up.
// A context error leaves shutdown in progress; repeated calls wait on the same
// completion and never report success while a task is still running.
func (scheduler *Scheduler) CloseContext(ctx context.Context) error {
	if scheduler == nil {
		return nil
	}
	scheduler.mu.Lock()
	if !scheduler.closed {
		scheduler.closed = true
		close(scheduler.closedCh)
		for _, job := range scheduler.active {
			job.cancel()
		}
		close(scheduler.jobs)
		// Park adds workers under mu and rejects a closed scheduler, so no
		// WaitGroup additions can race this wait after admission stops.
		go func() {
			scheduler.wg.Wait()
			close(scheduler.done)
		}()
	}
	done := scheduler.done
	scheduler.mu.Unlock()

	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (scheduler *Scheduler) worker() {
	defer scheduler.wg.Done()
	for job := range scheduler.jobs {
		scheduler.runJob(job)
	}
}

func (scheduler *Scheduler) substituteWorker(unpark <-chan struct{}) {
	defer scheduler.wg.Done()
	select {
	case job, ok := <-scheduler.jobs:
		if !ok {
			return
		}
		scheduler.runJob(job)
	case <-unpark:
	case <-scheduler.closedCh:
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
