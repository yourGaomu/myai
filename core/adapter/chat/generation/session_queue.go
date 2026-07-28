package generation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	runtimeservice "myai/core/application/runtime/service"
)

type sessionTaskQueue struct {
	running bool
	tasks   []func()
}

// SessionQueue serializes persistence tasks for one Session while allowing
// unrelated Sessions to use the shared worker pool concurrently.
type SessionQueue struct {
	Async   runtimeservice.AsyncTaskService
	OnPanic func(error)

	mu     sync.Mutex
	queues map[string]*sessionTaskQueue
}

func NewSessionQueue(async runtimeservice.AsyncTaskService) *SessionQueue {
	return &SessionQueue{Async: async, queues: make(map[string]*sessionTaskQueue)}
}

func (q *SessionQueue) Submit(sessionID string, task func()) error {
	if q == nil || task == nil {
		return errors.New("session queue and task are required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return q.Async.Submit(func() { q.runTask("", task) })
	}

	q.mu.Lock()
	if q.queues == nil {
		q.queues = make(map[string]*sessionTaskQueue)
	}
	queue := q.queues[sessionID]
	if queue == nil {
		queue = &sessionTaskQueue{}
		q.queues[sessionID] = queue
	}
	queue.tasks = append(queue.tasks, task)
	if queue.running {
		q.mu.Unlock()
		return nil
	}
	queue.running = true
	q.mu.Unlock()

	if err := q.Async.Submit(func() { q.drain(sessionID, queue) }); err != nil {
		q.mu.Lock()
		if q.queues[sessionID] == queue {
			delete(q.queues, sessionID)
			queue.running = false
			queue.tasks = nil
		}
		q.mu.Unlock()
		return err
	}
	return nil
}

func (q *SessionQueue) SubmitAndWait(ctx context.Context, sessionID string, task func() error) error {
	if task == nil {
		return errors.New("session queue task is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(chan error, 1)
	if err := q.Submit(sessionID, func() {
		var taskErr error
		defer func() {
			if recovered := recover(); recovered != nil {
				taskErr = fmt.Errorf("session persistence task panicked for %q: %v", sessionID, recovered)
			}
			result <- taskErr
		}()
		taskErr = task()
	}); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *SessionQueue) drain(sessionID string, queue *sessionTaskQueue) {
	for {
		q.mu.Lock()
		current := q.queues[sessionID]
		if current != queue || len(queue.tasks) == 0 {
			if current == queue {
				delete(q.queues, sessionID)
			}
			queue.running = false
			q.mu.Unlock()
			return
		}
		task := queue.tasks[0]
		queue.tasks[0] = nil
		queue.tasks = queue.tasks[1:]
		q.mu.Unlock()

		q.runTask(sessionID, task)
	}
}

func (q *SessionQueue) runTask(sessionID string, task func()) {
	defer func() {
		if recovered := recover(); recovered != nil && q.OnPanic != nil {
			q.OnPanic(fmt.Errorf("session persistence task panicked for %q: %v", sessionID, recovered))
		}
	}()
	task()
}
