package generation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	runtimeservice "myai/core/application/runtime/service"
	outbox "myai/core/port/outbox"
	repository "myai/core/port/repository"
)

type queuedSessionTask struct {
	run      func() error
	result   chan error
	started  bool
	assets   []repository.AssetRecord
	sequence int64
}

type sessionTaskQueue struct {
	running bool
	tasks   []*queuedSessionTask
	err     error
}

// SessionQueue preserves submission order and retains a failed head for retry.
type SessionQueue struct {
	Async   runtimeservice.AsyncTaskService
	OnPanic func(error)
	OnError func(error)

	mu      sync.Mutex
	queues  map[string]*sessionTaskQueue
	changed chan struct{}

	journal outbox.Journal
	sink    outbox.Sink
	blocked map[string]bool
	active  map[string]bool
}

func NewSessionQueue(async runtimeservice.AsyncTaskService) *SessionQueue {
	return &SessionQueue{Async: async, queues: make(map[string]*sessionTaskQueue)}
}

func (q *SessionQueue) Submit(sessionID string, task func()) error {
	if task == nil {
		return errors.New("session queue task is required")
	}
	return q.SubmitTask(sessionID, func() error { task(); return nil })
}

func (q *SessionQueue) SubmitTask(sessionID string, task func() error) error {
	if q == nil || task == nil {
		return errors.New("session queue and task are required")
	}
	if q.journal != nil {
		return outbox.ErrClosureNotDurable
	}
	return q.submitEntry(sessionID, &queuedSessionTask{run: task})
}

func (q *SessionQueue) submitEntry(sessionID string, task *queuedSessionTask) error {
	if q == nil || task == nil || task.run == nil {
		return errors.New("session queue and task are required")
	}
	sessionID = strings.TrimSpace(sessionID)
	q.mu.Lock()
	if q.queues == nil {
		q.queues = make(map[string]*sessionTaskQueue)
	}
	if q.blocked[sessionID] {
		q.mu.Unlock()
		return outbox.ErrBlocked
	}
	if q.journal != nil {
		blocked, err := q.journal.Blocked(context.Background(), sessionID)
		if err != nil {
			q.mu.Unlock()
			return err
		}
		if blocked {
			q.mu.Unlock()
			return outbox.ErrBlocked
		}
	}
	queue := q.queues[sessionID]
	if task.result != nil && queue != nil && queue.err != nil {
		err := fmt.Errorf("earlier persistence task failed: %w", queue.err)
		q.mu.Unlock()
		return err
	}
	if queue == nil {
		queue = &sessionTaskQueue{}
		q.queues[sessionID] = queue
	}
	queue.tasks = append(queue.tasks, task)
	start := !queue.running
	if start {
		queue.running, queue.err = true, nil
	}
	q.notifyLocked()
	q.mu.Unlock()
	if start {
		q.start(sessionID, queue)
	}
	return nil
}

func (q *SessionQueue) start(sessionID string, queue *sessionTaskQueue) {
	if err := q.Async.Submit(func() { q.drain(sessionID, queue) }); err != nil {
		// AsyncTaskService already implements QueueFull caller-runs. A closed
		// executor still must not discard work accepted by this queue.
		q.drain(sessionID, queue)
	}
}

func (q *SessionQueue) SubmitAndWait(ctx context.Context, sessionID string, task func() error) error {
	if task == nil {
		return errors.New("session queue task is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	entry := &queuedSessionTask{run: task, result: make(chan error, 1)}
	if err := q.submitEntry(sessionID, entry); err != nil {
		return err
	}
	select {
	case err := <-entry.result:
		return err
	case <-ctx.Done():
		q.mu.Lock()
		if !entry.started {
			if queue := q.queues[strings.TrimSpace(sessionID)]; queue != nil {
				for index, queued := range queue.tasks {
					if queued == entry {
						queue.tasks = append(queue.tasks[:index], queue.tasks[index+1:]...)
						q.notifyLocked()
						break
					}
				}
			}
			q.mu.Unlock()
			return ctx.Err()
		}
		q.mu.Unlock()
		// Once execution started, wait for a definite outcome before rollback.
		return <-entry.result
	}
}

func (q *SessionQueue) drain(sessionID string, queue *sessionTaskQueue) {
	for {
		q.mu.Lock()
		if q.queues[sessionID] != queue || len(queue.tasks) == 0 || q.blocked[sessionID] {
			if q.queues[sessionID] == queue && len(queue.tasks) == 0 {
				delete(q.queues, sessionID)
			}
			queue.running = false
			q.notifyLocked()
			q.mu.Unlock()
			return
		}
		task := queue.tasks[0]
		task.started = true
		if q.active == nil {
			q.active = make(map[string]bool)
		}
		q.active[sessionID] = true
		q.mu.Unlock()
		err := q.runTask(sessionID, task.run)
		q.mu.Lock()
		delete(q.active, sessionID)
		if task.result != nil {
			task.result <- err
			// Synchronous mutations own rollback and are never retried later.
			err = nil
		}
		if err != nil {
			queue.err, queue.running = err, false
			retained := queue.tasks[:1]
			for _, pending := range queue.tasks[1:] {
				if pending.result != nil {
					pending.started = true
					pending.result <- fmt.Errorf("earlier persistence task failed: %w", err)
				} else {
					retained = append(retained, pending)
				}
			}
			queue.tasks = retained
			q.notifyLocked()
			q.mu.Unlock()
			if q.OnError != nil {
				q.OnError(err)
			}
			return
		}
		queue.tasks[0] = nil
		queue.tasks = queue.tasks[1:]
		q.notifyLocked()
		q.mu.Unlock()
	}
}

// Flush retries each paused session once and waits for accepted work. A failure
// remains queued and is returned, so another Flush can retry the same identities.
func (q *SessionQueue) Flush(ctx context.Context) error {
	if q == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	type worker struct {
		id    string
		queue *sessionTaskQueue
	}
	q.mu.Lock()
	var workers []worker
	for id, queue := range q.queues {
		if !queue.running && len(queue.tasks) > 0 && !q.blocked[id] {
			queue.running, queue.err = true, nil
			workers = append(workers, worker{id, queue})
		}
	}
	q.mu.Unlock()
	for _, worker := range workers {
		q.start(worker.id, worker.queue)
	}
	for {
		q.mu.Lock()
		var errs []error
		running := false
		for id, queue := range q.queues {
			running = running || queue.running
			if queue.err != nil {
				errs = append(errs, fmt.Errorf("session %s persistence pending: %w", id, queue.err))
			}
		}
		if !running {
			for _, queue := range q.queues {
				if len(queue.tasks) > 0 && queue.err == nil {
					errs = append(errs, outbox.ErrBlocked)
				}
			}
			q.mu.Unlock()
			if q.journal != nil {
				stats, err := q.journal.Stats(ctx)
				if err != nil {
					errs = append(errs, err)
				} else if stats.Invalid > 0 {
					errs = append(errs, outbox.ErrInvalidRecord)
				}
			}
			return errors.Join(errs...)
		}
		if q.changed == nil {
			q.changed = make(chan struct{})
		}
		changed := q.changed
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (q *SessionQueue) notifyLocked() {
	if q.changed != nil {
		close(q.changed)
	}
	q.changed = make(chan struct{})
}

func (q *SessionQueue) runTask(sessionID string, task func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			// Backend panics can contain message bodies or connection credentials.
			err = errors.New("session persistence task panicked")
			if q.OnPanic != nil {
				q.OnPanic(err)
			}
		}
	}()
	return task()
}
