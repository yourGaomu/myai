package threadpool

import (
	"errors"
	"sync"
)

type Pool struct {
	workers int
	tasks   chan func()
	wg      sync.WaitGroup
	closed  bool
	mu      sync.Mutex
}

func New(workers int, queueSize int) *Pool {
	pool := &Pool{
		workers: workers,
		tasks:   make(chan func(), queueSize),
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
		return errors.New("thread pool is closed")
	}
	select {
	case p.tasks <- task:
		return nil
	default:
		return errors.New("task queue is full")
	}
}

func (p *Pool) Shutdown() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	close(p.tasks)
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for task := range p.tasks {
		task()
	}
}
