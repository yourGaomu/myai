package grpcprocessor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrWorkerPoolClosed = errors.New("document processor worker pool is closed")
	ErrWorkerQueueFull  = errors.New("document processor worker queue is full")
)

type workerPool struct {
	config     Config
	supervisor *workerSupervisor
	available  chan *worker
	admission  chan struct{}
	done       chan struct{}
	mu         sync.Mutex
	closed     bool
	workers    map[string]*worker
	replaceWG  sync.WaitGroup
	closeOnce  sync.Once
}

func newWorkerPool(ctx context.Context, config Config) (*workerPool, error) {
	pool := &workerPool{
		config:     config,
		supervisor: newWorkerSupervisor(config),
		available:  make(chan *worker, config.WorkerCount),
		admission:  make(chan struct{}, config.WorkerCount+config.MaxPendingJobs),
		done:       make(chan struct{}),
		workers:    make(map[string]*worker, config.WorkerCount),
	}
	startupContext, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan workerStartResult, config.WorkerCount)
	for index := 0; index < config.WorkerCount; index++ {
		go func(index int) {
			value, err := pool.supervisor.Start(startupContext, index)
			results <- workerStartResult{index: index, worker: value, err: err}
		}(index)
	}
	started := make([]*worker, 0, config.WorkerCount)
	var startErr error
	for range config.WorkerCount {
		result := <-results
		if result.err != nil {
			if startErr == nil {
				startErr = fmt.Errorf("start document processor worker %d: %w", result.index+1, result.err)
				cancel()
			}
			continue
		}
		started = append(started, result.worker)
	}
	if startErr != nil {
		for _, value := range started {
			_ = value.Close()
		}
		return nil, startErr
	}
	for _, value := range started {
		pool.workers[value.id] = value
		pool.available <- value
	}
	return pool, nil
}

type workerStartResult struct {
	index  int
	worker *worker
	err    error
}

func (pool *workerPool) acquire(ctx context.Context) (*worker, func(bool), error) {
	select {
	case <-pool.done:
		return nil, nil, ErrWorkerPoolClosed
	case pool.admission <- struct{}{}:
	default:
		return nil, nil, ErrWorkerQueueFull
	}

	select {
	case <-pool.done:
		<-pool.admission
		return nil, nil, ErrWorkerPoolClosed
	case <-ctx.Done():
		<-pool.admission
		return nil, nil, ctx.Err()
	case acquired := <-pool.available:
		var once sync.Once
		return acquired, func(reusable bool) {
			once.Do(func() {
				<-pool.admission
				pool.release(acquired, reusable)
			})
		}, nil
	}
}

func (pool *workerPool) release(value *worker, reusable bool) {
	select {
	case <-pool.done:
		_ = value.Close()
		return
	default:
	}
	if reusable {
		select {
		case pool.available <- value:
		case <-pool.done:
			_ = value.Close()
		}
		return
	}
	pool.mu.Lock()
	if pool.closed {
		pool.mu.Unlock()
		_ = value.Close()
		return
	}
	pool.replaceWG.Add(1)
	pool.mu.Unlock()
	go func() {
		defer pool.replaceWG.Done()
		pool.replace(value)
	}()
}

func (pool *workerPool) replace(previous *worker) {
	pool.mu.Lock()
	delete(pool.workers, previous.id)
	pool.mu.Unlock()
	_ = previous.Close()

	for attempt := 0; ; attempt++ {
		select {
		case <-pool.done:
			return
		default:
		}
		startupContext, cancel := context.WithTimeout(context.Background(), pool.config.StartupTimeout)
		replacement, err := pool.supervisor.Start(startupContext, attempt)
		cancel()
		if err == nil {
			pool.mu.Lock()
			if pool.closed {
				pool.mu.Unlock()
				_ = replacement.Close()
				return
			}
			pool.workers[replacement.id] = replacement
			pool.mu.Unlock()
			select {
			case pool.available <- replacement:
			case <-pool.done:
				_ = replacement.Close()
			}
			return
		}
		select {
		case <-pool.done:
			return
		case <-time.After(time.Second):
		}
	}
}

func (pool *workerPool) Close() error {
	var closeErr error
	pool.closeOnce.Do(func() {
		pool.mu.Lock()
		pool.closed = true
		close(pool.done)
		workers := make([]*worker, 0, len(pool.workers))
		for _, value := range pool.workers {
			workers = append(workers, value)
		}
		pool.workers = map[string]*worker{}
		pool.mu.Unlock()
		for _, value := range workers {
			closeErr = errors.Join(closeErr, value.Close())
		}
		pool.replaceWG.Wait()
	})
	return closeErr
}
