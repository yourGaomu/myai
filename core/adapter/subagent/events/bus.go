package events

import (
	"context"
	"strings"
	"sync"
	"time"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

const (
	TaskEventKindUpdated   = subagentport.TaskEventKindUpdated
	TaskEventKindCompleted = subagentport.TaskEventKindCompleted
	TaskEventKindCanceled  = subagentport.TaskEventKindCanceled
	TaskEventKindFailed    = subagentport.TaskEventKindFailed
)

type eventSubscriber struct {
	parentSessionID string
	channel         chan subagentport.TaskEvent
}

type Bus struct {
	mu               sync.RWMutex
	nextID           uint64
	nextSequence     uint64
	subscribers      map[uint64]chan domainsubagent.Task
	eventSubscribers map[uint64]eventSubscriber
	history          []subagentport.TaskEvent
	historyLimit     int
	eventRepository  subagentport.TaskEventRepository
}

var _ subagentport.EventPublisher = (*Bus)(nil)
var _ subagentport.TaskEventSource = (*Bus)(nil)

func NewBus(repositories ...subagentport.TaskEventRepository) *Bus {
	var repository subagentport.TaskEventRepository
	if len(repositories) > 0 {
		repository = repositories[0]
	}
	bus := &Bus{
		subscribers:      make(map[uint64]chan domainsubagent.Task),
		eventSubscribers: make(map[uint64]eventSubscriber),
		historyLimit:     512,
		eventRepository:  repository,
	}
	if repository != nil {
		// Recover the replay window and continue the persisted sequence after a
		// process restart. A repository failure must not prevent chat startup;
		// the live bus remains usable and future events can still be delivered.
		afterSequence := uint64(0)
		if tail, ok := repository.(subagentport.TaskEventTailRepository); ok {
			if latest, err := tail.LatestTaskEventSequence(context.Background()); err == nil && latest > uint64(bus.historyLimit) {
				afterSequence = latest - uint64(bus.historyLimit)
			}
		}
		if persisted, err := repository.ListTaskEvents(context.Background(), "", afterSequence, bus.historyLimit); err == nil {
			for _, event := range persisted {
				if event.Sequence > bus.nextSequence {
					bus.nextSequence = event.Sequence
				}
				bus.history = append(bus.history, cloneTaskEvent(event))
			}
		}
	}
	return bus
}

func (bus *Bus) TaskUpdated(ctx context.Context, task domainsubagent.Task) {
	bus.PublishTaskEvent(ctx, subagentport.TaskEvent{Kind: taskEventKind(task.Status), Task: task})
}

func (bus *Bus) PublishTaskEvent(ctx context.Context, event subagentport.TaskEvent) {
	if bus == nil {
		return
	}
	if event.Kind == "" {
		event.Kind = taskEventKind(event.Task.Status)
	}
	event.Task = domainsubagent.CloneTask(event.Task)
	if event.EmittedAt.IsZero() {
		event.EmittedAt = time.Now().UTC()
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.nextSequence++
	event.Sequence = bus.nextSequence
	bus.history = append(bus.history, cloneTaskEvent(event))
	if len(bus.history) > bus.historyLimit {
		bus.history = bus.history[len(bus.history)-bus.historyLimit:]
	}
	if bus.eventRepository != nil {
		// Keep persistence inside the same critical section as sequence
		// assignment so two concurrent producers cannot reorder the durable log.
		if err := bus.eventRepository.SaveTaskEvent(contextWithoutCancel(ctx), cloneTaskEvent(event)); err != nil {
			// EventPublisher intentionally has no error return. A failed write is
			// still visible through the live stream; the next process can recover
			// from the last successfully persisted sequence.
		}
	}
	bus.publishTaskLocked(event.Task)
	bus.publishEventLocked(event)
}

func contextWithoutCancel(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithoutCancel(ctx)
}

func (bus *Bus) publishTaskLocked(task domainsubagent.Task) {
	for _, subscriber := range bus.subscribers {
		select {
		case subscriber <- domainsubagent.CloneTask(task):
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- domainsubagent.CloneTask(task):
			default:
			}
		}
	}
}

func (bus *Bus) publishEventLocked(event subagentport.TaskEvent) {
	for id, subscriber := range bus.eventSubscribers {
		if subscriber.parentSessionID != "" && subscriber.parentSessionID != event.Task.ParentSessionID {
			continue
		}
		select {
		case subscriber.channel <- cloneTaskEvent(event):
		default:
			// An event stream must not silently skip sequence numbers. Close a
			// slow subscriber so it can reconnect with its last sequence.
			close(subscriber.channel)
			delete(bus.eventSubscribers, id)
		}
	}
}

func (bus *Bus) SubscribeTaskEvents(parentSessionID string, afterSequence uint64, buffer int) (<-chan subagentport.TaskEvent, func()) {
	if bus == nil {
		channel := make(chan subagentport.TaskEvent)
		close(channel)
		return channel, func() {}
	}
	if buffer < 1 {
		buffer = 32
	}
	parentSessionID = strings.TrimSpace(parentSessionID)

	bus.mu.Lock()
	bus.nextID++
	id := bus.nextID
	if afterSequence > 0 && len(bus.history) > 0 && afterSequence+1 < bus.history[0].Sequence {
		// The requested cursor is older than the in-memory replay window. A
		// partial replay is unsafe because callers cannot distinguish it from a
		// complete stream, so force an explicit reconnect/full resync.
		channel := make(chan subagentport.TaskEvent)
		close(channel)
		bus.mu.Unlock()
		return channel, func() {}
	}
	replay := make([]subagentport.TaskEvent, 0)
	for _, event := range bus.history {
		if event.Sequence <= afterSequence {
			continue
		}
		if parentSessionID != "" && event.Task.ParentSessionID != parentSessionID {
			continue
		}
		replay = append(replay, cloneTaskEvent(event))
	}
	channel := make(chan subagentport.TaskEvent, buffer+len(replay))
	for _, event := range replay {
		channel <- event
	}
	bus.eventSubscribers[id] = eventSubscriber{parentSessionID: parentSessionID, channel: channel}
	bus.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			bus.mu.Lock()
			if subscriber, ok := bus.eventSubscribers[id]; ok {
				delete(bus.eventSubscribers, id)
				close(subscriber.channel)
			}
			bus.mu.Unlock()
		})
	}
	return channel, cancel
}

func (bus *Bus) Subscribe(buffer int) (<-chan domainsubagent.Task, func()) {
	if buffer < 1 {
		buffer = 16
	}
	bus.mu.Lock()
	bus.nextID++
	id := bus.nextID
	channel := make(chan domainsubagent.Task, buffer)
	bus.subscribers[id] = channel
	bus.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			bus.mu.Lock()
			delete(bus.subscribers, id)
			close(channel)
			bus.mu.Unlock()
		})
	}
	return channel, cancel
}

func taskEventKind(status domainsubagent.TaskStatus) string {
	switch status {
	case domainsubagent.TaskStatusSucceeded:
		return TaskEventKindCompleted
	case domainsubagent.TaskStatusFailed:
		return TaskEventKindFailed
	case domainsubagent.TaskStatusCanceled:
		return TaskEventKindCanceled
	default:
		return TaskEventKindUpdated
	}
}

func cloneTaskEvent(event subagentport.TaskEvent) subagentport.TaskEvent {
	event.Task = domainsubagent.CloneTask(event.Task)
	return event
}
