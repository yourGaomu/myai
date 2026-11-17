package threadpool

import (
	"context"
	"errors"
	"log"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	asyncport "myai/core/port/async"
)

const defaultShutdownTimeout = 30 * time.Second

type Pool struct {
	workers int
	tasks   chan func()
	wg      sync.WaitGroup
	done    chan struct{}
	closed  bool
	mu      sync.Mutex
}

func New(workers int, queueSize int) *Pool {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if queueSize < 0 {
		queueSize = runtime.NumCPU()
	}

	pool := &Pool{
		workers: workers,
		tasks:   make(chan func(), queueSize),
		done:    make(chan struct{}),
	}
	for index := 0; index < workers; index++ {
		pool.wg.Add(1)
		go pool.worker()
	}
	return pool
}

func (p *Pool) Submit(task func()) error {
	if task == nil {
		return errors.New("task is nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return asyncport.ErrExecutorClosed
	}
	select {
	case p.tasks <- task:
		return nil
	default:
		return asyncport.ErrQueueFull
	}
}

// Shutdown stops admission and waits up to 30 seconds for accepted tasks.
// Call ShutdownContext to handle cancellation or a drain timeout explicitly.
func (p *Pool) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer cancel()
	if err := p.ShutdownContext(ctx); err != nil {
		log.Printf("thread pool shutdown incomplete: %v", err)
	}
}

// ShutdownContext stops admission and waits for all accepted tasks and workers.
// A context error means draining is still in progress; subsequent calls wait
// for the same completion. Submitted func() tasks cannot be forcibly canceled.
func (p *Pool) ShutdownContext(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.tasks)
		go func() {
			p.wg.Wait()
			close(p.done)
		}()
	}
	done := p.done
	p.mu.Unlock()

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

func (p *Pool) worker() {
	defer p.wg.Done()
	for task := range p.tasks {
		runTask(task)
	}
}

func runTask(task func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("thread pool task panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	task()
}
