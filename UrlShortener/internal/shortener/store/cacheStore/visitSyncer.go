package cacheStore

import (
	"context"
	"log"
	"sync"
	"time"

	"myai-url-shortener/internal/shortener/store"
)

type visitBatchIncrementer interface {
	IncrementVisitsBy(ctx context.Context, code string, delta int64) error
}

type VisitSyncerOptions struct {
	FlushInterval time.Duration
	QueueSize     int
}

type VisitSyncer struct {
	next          store.Store
	flushInterval time.Duration
	queue         chan string
	done          chan struct{}
	closeOnce     sync.Once
}

func NewVisitSyncer(next store.Store, options VisitSyncerOptions) *VisitSyncer {
	flushInterval := options.FlushInterval
	if flushInterval <= 0 {
		flushInterval = time.Second
	}
	queueSize := options.QueueSize
	if queueSize <= 0 {
		queueSize = 4096
	}

	syncer := &VisitSyncer{
		next:          next,
		flushInterval: flushInterval,
		queue:         make(chan string, queueSize),
		done:          make(chan struct{}),
	}
	go syncer.run()
	return syncer
}

func (s *VisitSyncer) Enqueue(code string) bool {
	select {
	case s.queue <- code:
		return true
	default:
		return false
	}
}

func (s *VisitSyncer) Close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		close(s.queue)
	})
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *VisitSyncer) run() {
	defer close(s.done)

	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	pending := make(map[string]int64)
	for {
		select {
		case code, ok := <-s.queue:
			if !ok {
				s.drain(pending)
				s.flush(pending)
				return
			}
			pending[code]++
		case <-ticker.C:
			s.flush(pending)
		}
	}
}

func (s *VisitSyncer) drain(pending map[string]int64) {
	for {
		select {
		case code, ok := <-s.queue:
			if !ok {
				return
			}
			pending[code]++
		default:
			return
		}
	}
}

func (s *VisitSyncer) flush(pending map[string]int64) {
	if len(pending) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for code, delta := range pending {
		if err := incrementVisitsBy(ctx, s.next, code, delta); err != nil {
			log.Printf("async mongo visits sync failed: code=%s delta=%d err=%v", code, delta, err)
		}
		delete(pending, code)
	}
}

func incrementVisitsBy(ctx context.Context, next store.Store, code string, delta int64) error {
	if delta <= 0 {
		return nil
	}
	if batcher, ok := next.(visitBatchIncrementer); ok {
		return batcher.IncrementVisitsBy(ctx, code, delta)
	}
	for i := int64(0); i < delta; i++ {
		if _, err := next.IncrementVisits(ctx, code); err != nil {
			return err
		}
	}
	return nil
}
