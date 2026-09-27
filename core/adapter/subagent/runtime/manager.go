package runtime

import (
	"sort"
	"strings"
	"sync"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

var _ subagentport.AgentRuntimeManager = (*Manager)(nil)

// Manager is the in-process runtime registry for child agents. It is
// deliberately independent from the scheduler: a thread can remain known
// after its current turn has finished, and can be unloaded without deleting
// its durable identity.
type Manager struct {
	mu      sync.RWMutex
	runtime map[string]subagentport.AgentRuntimeSnapshot
	turns   map[string]string
}

func New() *Manager {
	return &Manager{
		runtime: make(map[string]subagentport.AgentRuntimeSnapshot),
		turns:   make(map[string]string),
	}
}

func (manager *Manager) Register(thread domainsubagent.AgentThread) error {
	if manager == nil {
		return subagentport.ErrAgentRuntimeNotFound
	}
	threadID := strings.TrimSpace(thread.ID)
	if threadID == "" {
		return subagentport.ErrAgentRuntimeNotFound
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if current, ok := manager.runtime[threadID]; ok {
		if activeTurn := manager.turns[threadID]; activeTurn != "" && activeTurn != strings.TrimSpace(thread.CurrentRunID) {
			return subagentport.ErrAgentTurnActive
		}
		current.ParentThreadID = strings.TrimSpace(thread.ParentTaskID)
		current.RootAgentID = strings.TrimSpace(thread.ParentSessionID)
		current.AgentPath = normalizePath(thread.AgentPath)
		current.AgentNickname = strings.TrimSpace(thread.AgentNickname)
		current.CurrentTurnID = manager.turns[threadID]
		current.Status = statusForTask(thread.Status)
		manager.runtime[threadID] = current
		return nil
	}
	manager.runtime[threadID] = subagentport.AgentRuntimeSnapshot{
		ThreadID:       threadID,
		ParentThreadID: strings.TrimSpace(thread.ParentTaskID),
		RootAgentID:    strings.TrimSpace(thread.ParentSessionID),
		AgentPath:      normalizePath(thread.AgentPath),
		AgentNickname:  strings.TrimSpace(thread.AgentNickname),
		CurrentTurnID:  "",
		Status:         statusForTask(thread.Status),
	}
	return nil
}

func (manager *Manager) Remove(threadID string) error {
	if manager == nil {
		return subagentport.ErrAgentRuntimeNotFound
	}
	threadID = strings.TrimSpace(threadID)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if _, ok := manager.runtime[threadID]; !ok {
		return subagentport.ErrAgentRuntimeNotFound
	}
	if manager.turns[threadID] != "" {
		return subagentport.ErrAgentTurnActive
	}
	delete(manager.runtime, threadID)
	return nil
}

func (manager *Manager) ReserveTurn(threadID, turnID string) error {
	if manager == nil {
		return subagentport.ErrAgentRuntimeNotFound
	}
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	if threadID == "" || turnID == "" {
		return subagentport.ErrAgentRuntimeNotFound
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	state, ok := manager.runtime[threadID]
	if !ok {
		return subagentport.ErrAgentRuntimeNotFound
	}
	if activeTurn := manager.turns[threadID]; activeTurn != "" && activeTurn != turnID {
		return subagentport.ErrAgentTurnActive
	}
	manager.turns[threadID] = turnID
	state.CurrentTurnID = turnID
	state.Status = domainsubagent.AgentStatusQueued
	manager.runtime[threadID] = state
	return nil
}

func (manager *Manager) ReleaseTurn(threadID, turnID string) error {
	if manager == nil {
		return subagentport.ErrAgentRuntimeNotFound
	}
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if _, ok := manager.runtime[threadID]; !ok {
		return subagentport.ErrAgentRuntimeNotFound
	}
	activeTurn := manager.turns[threadID]
	if activeTurn == "" {
		return nil
	}
	if activeTurn != turnID {
		return subagentport.ErrAgentTurnMismatch
	}
	delete(manager.turns, threadID)
	state := manager.runtime[threadID]
	state.CurrentTurnID = ""
	manager.runtime[threadID] = state
	return nil
}

func (manager *Manager) SetStatus(threadID string, status domainsubagent.AgentStatus) error {
	if manager == nil {
		return subagentport.ErrAgentRuntimeNotFound
	}
	threadID = strings.TrimSpace(threadID)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	state, ok := manager.runtime[threadID]
	if !ok {
		return subagentport.ErrAgentRuntimeNotFound
	}
	state.Status = status
	manager.runtime[threadID] = state
	return nil
}

func (manager *Manager) MarkLoaded(threadID string) error {
	if manager == nil {
		return subagentport.ErrAgentRuntimeNotFound
	}
	threadID = strings.TrimSpace(threadID)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	state, ok := manager.runtime[threadID]
	if !ok {
		return subagentport.ErrAgentRuntimeNotFound
	}
	if state.Loaded {
		return subagentport.ErrAgentRuntimeLoaded
	}
	state.Loaded = true
	if state.Status == domainsubagent.AgentStatusUnloaded {
		state.Status = domainsubagent.AgentStatusIdle
	}
	manager.runtime[threadID] = state
	return nil
}

func (manager *Manager) MarkUnloaded(threadID string) error {
	if manager == nil {
		return subagentport.ErrAgentRuntimeNotFound
	}
	threadID = strings.TrimSpace(threadID)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	state, ok := manager.runtime[threadID]
	if !ok {
		return subagentport.ErrAgentRuntimeNotFound
	}
	if manager.turns[threadID] != "" {
		return subagentport.ErrAgentTurnActive
	}
	if !state.Loaded {
		return subagentport.ErrAgentRuntimeUnloaded
	}
	state.Loaded = false
	state.Status = domainsubagent.AgentStatusUnloaded
	manager.runtime[threadID] = state
	return nil
}

func (manager *Manager) Inspect(threadID string) (subagentport.AgentRuntimeSnapshot, error) {
	if manager == nil {
		return subagentport.AgentRuntimeSnapshot{}, subagentport.ErrAgentRuntimeNotFound
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	state, ok := manager.runtime[strings.TrimSpace(threadID)]
	if !ok {
		return subagentport.AgentRuntimeSnapshot{}, subagentport.ErrAgentRuntimeNotFound
	}
	return state, nil
}

func (manager *Manager) List() []subagentport.AgentRuntimeSnapshot {
	if manager == nil {
		return nil
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	items := make([]subagentport.AgentRuntimeSnapshot, 0, len(manager.runtime))
	for _, state := range manager.runtime {
		items = append(items, state)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].AgentPath == items[j].AgentPath {
			return items[i].ThreadID < items[j].ThreadID
		}
		return items[i].AgentPath < items[j].AgentPath
	})
	return items
}

func statusForTask(status domainsubagent.TaskStatus) domainsubagent.AgentStatus {
	switch status {
	case domainsubagent.TaskStatusQueued:
		return domainsubagent.AgentStatusQueued
	case domainsubagent.TaskStatusRunning:
		return domainsubagent.AgentStatusRunning
	case domainsubagent.TaskStatusWaitingSubagents, domainsubagent.TaskStatusWaitingPermission:
		return domainsubagent.AgentStatusWaiting
	case domainsubagent.TaskStatusSucceeded:
		return domainsubagent.AgentStatusCompleted
	case domainsubagent.TaskStatusFailed:
		return domainsubagent.AgentStatusFailed
	case domainsubagent.TaskStatusCanceled:
		return domainsubagent.AgentStatusInterrupted
	default:
		return domainsubagent.AgentStatusCreated
	}
}

func normalizePath(path string) string {
	return strings.Trim(strings.TrimSpace(path), "/")
}
