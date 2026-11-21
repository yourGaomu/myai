package generation

import (
	"context"
	"errors"

	runtimeservice "myai/core/application/runtime/service"
	outbox "myai/core/port/outbox"
	repository "myai/core/port/repository"
)

// NewDurableSessionQueue loads and validates recovery before starting any work.
// The caller owns journal.Close, including when construction fails. Recovery
// uses only typed records and does not require in-memory sessions or models.
func NewDurableSessionQueue(async runtimeservice.AsyncTaskService, journal outbox.Journal, sink outbox.Sink) (*SessionQueue, error) {
	if journal == nil || sink == nil {
		return nil, errors.New("durable session queue requires journal and sink")
	}
	entries, err := journal.Pending(context.Background())
	if err != nil {
		return nil, err
	}
	q := NewSessionQueue(async)
	q.journal, q.sink = journal, sink
	q.blocked = make(map[string]bool)
	for _, entry := range entries {
		if entry.State != outbox.Pending || entry.Sequence <= 0 || entry.Batch.Validate() != nil {
			return nil, errors.New("invalid durable queue recovery entry")
		}
		queue := q.queues[entry.Batch.SessionID]
		if queue == nil {
			queue = &sessionTaskQueue{}
			q.queues[entry.Batch.SessionID] = queue
		}
		queue.tasks = append(queue.tasks, q.recordTask(entry))
	}
	type worker struct {
		id    string
		queue *sessionTaskQueue
	}
	workers := make([]worker, 0, len(q.queues))
	for id, queue := range q.queues {
		queue.running = true
		workers = append(workers, worker{id, queue})
	}
	for _, worker := range workers {
		q.start(worker.id, worker.queue)
	}
	return q, nil
}

func (q *SessionQueue) Durable() bool { return q != nil && q.journal != nil }

// SubmitRecords returns success only once the typed record is durably accepted.
// Backend failure after that point leaves a visible pending head for Flush or
// restart replay. No closure or synchronous replacement is persisted.
func (q *SessionQueue) SubmitRecords(ctx context.Context, batch outbox.Batch) error {
	if !q.Durable() {
		return errors.New("durable session queue is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	q.mu.Lock()
	if q.blocked[batch.SessionID] {
		q.mu.Unlock()
		return outbox.ErrBlocked
	}
	entry, err := q.journal.Append(ctx, batch)
	if err != nil {
		q.mu.Unlock()
		return err
	}
	if entry.State == outbox.Done {
		q.mu.Unlock()
		return nil
	}
	queue := q.queues[entry.Batch.SessionID]
	if queue == nil {
		queue = &sessionTaskQueue{}
		q.queues[entry.Batch.SessionID] = queue
	}
	for _, task := range queue.tasks {
		if task.sequence == entry.Sequence {
			q.mu.Unlock()
			return nil
		}
	}
	queue.tasks = append(queue.tasks, q.recordTask(entry))
	start := !queue.running
	if start {
		queue.running, queue.err = true, nil
	}
	q.notifyLocked()
	q.mu.Unlock()
	if start {
		q.start(entry.Batch.SessionID, queue)
	}
	return nil
}

// PendingSessionAttachment includes durable receipts before the backend write
// completes. Acknowledged assets are already visible through the repository.
func (q *SessionQueue) PendingSessionAttachment(ctx context.Context, sessionID, assetID string) (repository.AssetRecord, bool, error) {
	if q == nil {
		return repository.AssetRecord{}, false, nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.blocked[sessionID] {
		return repository.AssetRecord{}, false, outbox.ErrBlocked
	}
	if q.journal != nil {
		blocked, err := q.journal.Blocked(ctx, sessionID)
		if err != nil {
			return repository.AssetRecord{}, false, err
		}
		if blocked {
			return repository.AssetRecord{}, false, outbox.ErrBlocked
		}
	}
	if queue := q.queues[sessionID]; queue != nil {
		for _, task := range queue.tasks {
			for _, asset := range task.assets {
				if asset.AssetID == assetID && asset.SessionID == sessionID && asset.IsSessionAttachment() {
					return asset, true, nil
				}
			}
		}
	}
	return repository.AssetRecord{}, false, nil
}

func (q *SessionQueue) recordTask(entry outbox.Entry) *queuedSessionTask {
	return &queuedSessionTask{assets: entry.Batch.Assets, sequence: entry.Sequence, run: func() error {
		ctx, cancel := context.WithTimeout(context.Background(), defaultPersistenceTimeout)
		defer cancel()
		if err := q.sink.Apply(ctx, entry.Batch); err != nil {
			return outbox.SafeError("apply durable persistence", err)
		}
		// Do not use the possibly expired backend context for the commit receipt.
		ackCtx, ackCancel := context.WithTimeout(context.Background(), defaultPersistenceTimeout)
		defer ackCancel()
		return q.journal.Ack(ackCtx, entry.Sequence)
	}}
}

// BlockSession is a per-session lifecycle fence. Stop and await producers before
// calling it. It waits for only the active backend attempt, never for a paused
// head or an executor that has not started. A committed tombstone cancels old
// pending work and survives restart. Delete keeps it; clear may explicitly Resume
// only after the synchronous backend mutation succeeds. Failure stays blocked.
func (q *SessionQueue) BlockSession(ctx context.Context, sessionID string) error {
	if q == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if sessionID == "" {
		return errors.New("session id is required")
	}
	q.mu.Lock()
	if q.blocked == nil {
		q.blocked = make(map[string]bool)
	}
	q.blocked[sessionID] = true
	q.notifyLocked()
	for q.active[sessionID] {
		changed := q.changed
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
		q.mu.Lock()
	}
	defer q.mu.Unlock()
	if q.journal != nil {
		if err := q.journal.Block(ctx, sessionID); err != nil {
			return err
		}
	}
	if queue := q.queues[sessionID]; queue != nil {
		for _, task := range queue.tasks {
			if task.result != nil {
				task.started = true
				task.result <- outbox.ErrBlocked
			}
		}
		delete(q.queues, sessionID)
	}
	q.notifyLocked()
	return nil
}

func (q *SessionQueue) ResumeSession(ctx context.Context, sessionID string) error {
	if q == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active[sessionID] || q.queues[sessionID] != nil {
		return errors.New("session persistence barrier is not settled")
	}
	if q.journal != nil {
		if err := q.journal.Resume(ctx, sessionID); err != nil {
			return err
		}
	}
	delete(q.blocked, sessionID)
	q.notifyLocked()
	return nil
}

func (q *SessionQueue) Stats(ctx context.Context) (outbox.Stats, error) {
	if q == nil || q.journal == nil {
		return outbox.Stats{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return q.journal.Stats(ctx)
}
