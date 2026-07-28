package events

import (
	"context"
	"sync"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

type Bus struct {
	mu          sync.RWMutex
	nextID      uint64
	subscribers map[uint64]chan domainsubagent.Task
}

var _ subagentport.EventPublisher = (*Bus)(nil)

func NewBus() *Bus {
	return &Bus{subscribers: make(map[uint64]chan domainsubagent.Task)}
}

func (bus *Bus) TaskUpdated(_ context.Context, task domainsubagent.Task) {
	if bus == nil {
		return
	}
	bus.mu.RLock()
	defer bus.mu.RUnlock()
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
