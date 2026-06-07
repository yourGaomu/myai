package utills

import (
	"errors"
	"sync"
)

type Task func()
type ThreadPool struct {
	core   int
	tasks  chan Task
	wg     sync.WaitGroup
	closed bool
	mu     sync.Mutex
}

func NewThreadPool(core int, queueSize int) *ThreadPool {
	pool := &ThreadPool{
		core:  core,
		tasks: make(chan Task, queueSize),
	}

	for i := 0; i < core; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}
	return pool
}

func (pool *ThreadPool) worker() {
	defer pool.wg.Done()

	for task := range pool.tasks {
		task()
	}
}

func (p *ThreadPool) Submit(task Task) error {
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

func (p *ThreadPool) Shutdown() {
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
