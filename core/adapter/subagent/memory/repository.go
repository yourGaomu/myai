package memory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

type Repository struct {
	mu            sync.RWMutex
	definitions   map[string]domainsubagent.Definition
	tasks         map[string]domainsubagent.Task
	runs          map[string][]domainsubagent.Run
	agentMessages map[string]domainsubagent.AgentMessage
	leases        map[string]executionLease
	events        []subagentport.TaskEvent
	eventSequence uint64
}

type executionLease struct {
	runID     string
	ownerID   string
	expiresAt time.Time
}

var _ subagentport.DefinitionRepository = (*Repository)(nil)
var _ subagentport.TaskRepository = (*Repository)(nil)
var _ subagentport.RunRepository = (*Repository)(nil)
var _ subagentport.TaskRunRepository = (*Repository)(nil)
var _ subagentport.InterruptedTaskRepository = (*Repository)(nil)
var _ subagentport.TaskEventRepository = (*Repository)(nil)
var _ subagentport.TaskEventTailRepository = (*Repository)(nil)
var _ subagentport.TaskEventSequenceRepository = (*Repository)(nil)
var _ subagentport.TaskEventSequenceInitializer = (*Repository)(nil)
var _ subagentport.AgentMessageRepository = (*Repository)(nil)
var _ subagentport.ParentCompletionMessageRepository = (*Repository)(nil)
var _ subagentport.ChildAgentMessageRepository = (*Repository)(nil)
var _ subagentport.AgentExecutionLeaseStore = (*Repository)(nil)
var _ subagentport.LeaseGuardedTaskRunRepository = (*Repository)(nil)
var _ subagentport.TaskCancellationRepository = (*Repository)(nil)
var _ subagentport.TaskVersionRepository = (*Repository)(nil)
var _ subagentport.LeaseGuardedTaskRepository = (*Repository)(nil)

func NewRepository() *Repository {
	return &Repository{
		definitions:   make(map[string]domainsubagent.Definition),
		tasks:         make(map[string]domainsubagent.Task),
		runs:          make(map[string][]domainsubagent.Run),
		agentMessages: make(map[string]domainsubagent.AgentMessage),
		leases:        make(map[string]executionLease),
		events:        make([]subagentport.TaskEvent, 0),
	}
}

func (repository *Repository) EnqueueChildAgentMessage(_ context.Context, taskID string, message domainsubagent.AgentMessage) (domainsubagent.Task, error) {
	if err := message.Validate(); err != nil {
		return domainsubagent.Task{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	task, ok := repository.tasks[strings.TrimSpace(taskID)]
	if !ok {
		return domainsubagent.Task{}, subagentport.ErrNotFound
	}
	if existing, exists := repository.agentMessages[message.ID]; exists {
		if !existing.SameRequest(message) {
			return domainsubagent.Task{}, errors.New("agent message id was already used with different request")
		}
		return domainsubagent.CloneTask(task), nil
	}
	for _, queued := range task.Mailbox {
		if queued.ID == message.ID {
			return domainsubagent.Task{}, errors.New("agent message id already exists in task mailbox")
		}
	}
	if err := task.EnqueueMessage(domainsubagent.Message{ID: message.ID, Content: message.Content, Trigger: message.Trigger, CreatedAt: message.CreatedAt}); err != nil {
		return domainsubagent.Task{}, err
	}
	repository.tasks[task.ID] = domainsubagent.CloneTask(task)
	repository.agentMessages[message.ID] = domainsubagent.CloneAgentMessage(message)
	return domainsubagent.CloneTask(task), nil
}

func (repository *Repository) ClaimChildMailboxMessage(_ context.Context, taskID string, now time.Time) (domainsubagent.Message, bool, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	task, ok := repository.tasks[strings.TrimSpace(taskID)]
	if !ok {
		return domainsubagent.Message{}, false, subagentport.ErrNotFound
	}
	message, claimed := task.ClaimMailboxMessage(now)
	if claimed {
		repository.tasks[task.ID] = domainsubagent.CloneTask(task)
		if envelope, exists := repository.agentMessages[message.ID]; exists {
			envelope.Status = domainsubagent.AgentMessageDelivering
			envelope.DeliveryAttempts = message.DeliveryAttempts
			envelope.LastError = ""
			repository.agentMessages[message.ID] = domainsubagent.CloneAgentMessage(envelope)
		}
	}
	return message, claimed, nil
}

func (repository *Repository) AcknowledgeChildAgentMessage(_ context.Context, taskID, messageID string, deliveredAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	task, ok := repository.tasks[strings.TrimSpace(taskID)]
	if !ok {
		return subagentport.ErrNotFound
	}
	message, ok := repository.agentMessages[strings.TrimSpace(messageID)]
	if !ok {
		return subagentport.ErrNotFound
	}
	if message.Status == domainsubagent.AgentMessageDelivered {
		return nil
	}
	if message.RecipientAgentID != task.ChildSessionID || !task.AcknowledgeMailboxMessage(messageID, deliveredAt) {
		return errors.New("claimed child agent message is missing from task mailbox")
	}
	message.Status = domainsubagent.AgentMessageDelivered
	message.DeliveredAt = &deliveredAt
	repository.tasks[task.ID] = domainsubagent.CloneTask(task)
	repository.agentMessages[message.ID] = domainsubagent.CloneAgentMessage(message)
	return nil
}

func (repository *Repository) ReleaseChildMailboxMessage(_ context.Context, taskID, messageID, reason string, now time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	task, ok := repository.tasks[strings.TrimSpace(taskID)]
	if !ok {
		return subagentport.ErrNotFound
	}
	var deliveryErr error
	if reason != "" {
		deliveryErr = errors.New(reason)
	}
	if task.ReleaseMailboxMessage(messageID, deliveryErr, now) {
		repository.tasks[task.ID] = domainsubagent.CloneTask(task)
		if envelope, exists := repository.agentMessages[messageID]; exists {
			envelope.Status = domainsubagent.AgentMessagePending
			envelope.LastError = reason
			repository.agentMessages[messageID] = domainsubagent.CloneAgentMessage(envelope)
		}
	}
	return nil
}

// Acquire implements a process-shared lease for tests and in-memory mode.
// A lease can be reacquired by its current owner, making recovery idempotent.
func (repository *Repository) Acquire(_ context.Context, taskID, runID, ownerID string, ttl time.Duration) error {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(runID) == "" || strings.TrimSpace(ownerID) == "" || ttl <= 0 {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	now := time.Now().UTC()
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.leases == nil {
		repository.leases = make(map[string]executionLease)
	}
	current, exists := repository.leases[taskID]
	if exists && current.expiresAt.After(now) && (current.runID != runID || current.ownerID != ownerID) {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	repository.leases[taskID] = executionLease{runID: runID, ownerID: ownerID, expiresAt: now.Add(ttl)}
	return nil
}

func (repository *Repository) Renew(_ context.Context, taskID, runID, ownerID string, ttl time.Duration) error {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(runID) == "" || strings.TrimSpace(ownerID) == "" || ttl <= 0 {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	now := time.Now().UTC()
	repository.mu.Lock()
	defer repository.mu.Unlock()
	current, exists := repository.leases[taskID]
	if !exists || !current.expiresAt.After(now) || current.runID != runID || current.ownerID != ownerID {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	current.expiresAt = now.Add(ttl)
	repository.leases[taskID] = current
	return nil
}

func (repository *Repository) Release(_ context.Context, taskID, runID, ownerID string) error {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(runID) == "" || strings.TrimSpace(ownerID) == "" {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	current, exists := repository.leases[taskID]
	if !exists {
		return nil
	}
	if current.runID != runID || current.ownerID != ownerID {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	delete(repository.leases, taskID)
	return nil
}

func (repository *Repository) SaveAgentMessage(_ context.Context, message domainsubagent.AgentMessage) error {
	if err := message.Validate(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.agentMessages == nil {
		repository.agentMessages = make(map[string]domainsubagent.AgentMessage)
	}
	repository.agentMessages[message.ID] = domainsubagent.CloneAgentMessage(message)
	return nil
}

func (repository *Repository) GetAgentMessage(_ context.Context, messageID string) (domainsubagent.AgentMessage, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	message, ok := repository.agentMessages[strings.TrimSpace(messageID)]
	if !ok {
		return domainsubagent.AgentMessage{}, subagentport.ErrNotFound
	}
	return domainsubagent.CloneAgentMessage(message), nil
}

func (repository *Repository) ListPendingAgentMessages(_ context.Context, recipientAgentID string, limit int) ([]domainsubagent.AgentMessage, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	recipientAgentID = strings.TrimSpace(recipientAgentID)
	items := make([]domainsubagent.AgentMessage, 0)
	for _, message := range repository.agentMessages {
		if recipientAgentID != "" && message.RecipientAgentID != recipientAgentID {
			continue
		}
		if message.Status != domainsubagent.AgentMessagePending && message.Status != domainsubagent.AgentMessageDelivering {
			continue
		}
		items = append(items, domainsubagent.CloneAgentMessage(message))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (repository *Repository) ListPendingParentMessages(_ context.Context) ([]domainsubagent.AgentMessage, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainsubagent.AgentMessage, 0)
	for _, message := range repository.agentMessages {
		if message.Status != domainsubagent.AgentMessagePending && message.Status != domainsubagent.AgentMessageDelivering {
			continue
		}
		if message.Kind != domainsubagent.AgentMessageKindTaskResult && message.Kind != domainsubagent.AgentMessageKindTaskError {
			continue
		}
		items = append(items, domainsubagent.CloneAgentMessage(message))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (repository *Repository) MarkAgentMessageDelivered(_ context.Context, messageID string, deliveredAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	message, ok := repository.agentMessages[strings.TrimSpace(messageID)]
	if !ok {
		return subagentport.ErrNotFound
	}
	message.Status = domainsubagent.AgentMessageDelivered
	message.DeliveredAt = &deliveredAt
	message.ClaimOwnerID = ""
	message.ClaimExpiresAt = nil
	repository.agentMessages[message.ID] = domainsubagent.CloneAgentMessage(message)
	return nil
}

func (repository *Repository) MarkAgentMessageFailed(_ context.Context, messageID string, reason string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	message, ok := repository.agentMessages[strings.TrimSpace(messageID)]
	if !ok {
		return subagentport.ErrNotFound
	}
	message.Status = domainsubagent.AgentMessageFailed
	message.LastError = strings.TrimSpace(reason)
	repository.agentMessages[message.ID] = domainsubagent.CloneAgentMessage(message)
	return nil
}

func (repository *Repository) EnqueueParentCompletion(_ context.Context, taskID string, message domainsubagent.AgentMessage) (domainsubagent.AgentMessage, error) {
	if err := message.Validate(); err != nil {
		return domainsubagent.AgentMessage{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	task, ok := repository.tasks[strings.TrimSpace(taskID)]
	if !ok {
		return domainsubagent.AgentMessage{}, subagentport.ErrNotFound
	}
	if !task.Terminal() || (message.Kind == domainsubagent.AgentMessageKindTaskResult && task.Status != domainsubagent.TaskStatusSucceeded) ||
		(message.Kind == domainsubagent.AgentMessageKindTaskError && task.Status != domainsubagent.TaskStatusFailed) {
		return domainsubagent.AgentMessage{}, subagentport.ErrTaskStateConflict
	}
	if existing, exists := repository.agentMessages[message.ID]; exists {
		if existing.SourceTaskID != taskID || existing.RecipientAgentID != task.ParentSessionID || existing.Kind != message.Kind || existing.Content != message.Content {
			return domainsubagent.AgentMessage{}, errors.New("parent completion message conflicts with existing envelope")
		}
		return domainsubagent.CloneAgentMessage(existing), nil
	}
	repository.agentMessages[message.ID] = domainsubagent.CloneAgentMessage(message)
	return domainsubagent.CloneAgentMessage(message), nil
}

func (repository *Repository) ClaimParentCompletion(_ context.Context, messageID, ownerID string, now time.Time, ttl time.Duration) (domainsubagent.AgentMessage, error) {
	if strings.TrimSpace(ownerID) == "" || ttl <= 0 {
		return domainsubagent.AgentMessage{}, errors.New("parent completion claim is invalid")
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	message, ok := repository.agentMessages[strings.TrimSpace(messageID)]
	if !ok {
		return domainsubagent.AgentMessage{}, subagentport.ErrNotFound
	}
	if message.Status == domainsubagent.AgentMessageDelivered {
		return domainsubagent.AgentMessage{}, subagentport.ErrTaskStateConflict
	}
	if message.Status == domainsubagent.AgentMessageDelivering && message.ClaimExpiresAt != nil && message.ClaimExpiresAt.After(now) && message.ClaimOwnerID != ownerID {
		return domainsubagent.AgentMessage{}, subagentport.ErrExecutionLeaseNotAcquired
	}
	expires := now.Add(ttl)
	message.Status = domainsubagent.AgentMessageDelivering
	message.ClaimOwnerID = ownerID
	message.ClaimExpiresAt = &expires
	message.DeliveryAttempts++
	message.LastError = ""
	repository.agentMessages[message.ID] = domainsubagent.CloneAgentMessage(message)
	return domainsubagent.CloneAgentMessage(message), nil
}

func (repository *Repository) ReleaseParentCompletion(_ context.Context, messageID, ownerID, reason string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	message, ok := repository.agentMessages[strings.TrimSpace(messageID)]
	if !ok {
		return subagentport.ErrNotFound
	}
	if message.Status != domainsubagent.AgentMessageDelivering || message.ClaimOwnerID != ownerID {
		return subagentport.ErrTaskStateConflict
	}
	message.Status = domainsubagent.AgentMessagePending
	message.ClaimOwnerID = ""
	message.ClaimExpiresAt = nil
	message.LastError = strings.TrimSpace(reason)
	repository.agentMessages[message.ID] = domainsubagent.CloneAgentMessage(message)
	return nil
}

func (repository *Repository) CompleteParentCompletion(_ context.Context, messageID, ownerID string, deliveredAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	message, ok := repository.agentMessages[strings.TrimSpace(messageID)]
	if !ok {
		return subagentport.ErrNotFound
	}
	if message.Status != domainsubagent.AgentMessageDelivering || message.ClaimOwnerID != ownerID {
		return subagentport.ErrTaskStateConflict
	}
	message.Status = domainsubagent.AgentMessageDelivered
	message.DeliveredAt = &deliveredAt
	message.ClaimOwnerID = ""
	message.ClaimExpiresAt = nil
	repository.agentMessages[message.ID] = domainsubagent.CloneAgentMessage(message)
	return nil
}

func (repository *Repository) SaveDefinition(_ context.Context, definition domainsubagent.Definition) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.definitions[definition.ID] = domainsubagent.CloneDefinition(definition)
	return nil
}

func (repository *Repository) GetDefinition(_ context.Context, definitionID string) (domainsubagent.Definition, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	definition, ok := repository.definitions[strings.TrimSpace(definitionID)]
	if !ok {
		return domainsubagent.Definition{}, subagentport.ErrNotFound
	}
	return domainsubagent.CloneDefinition(definition), nil
}

func (repository *Repository) ListDefinitions(_ context.Context, includeDeleted bool) ([]domainsubagent.Definition, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainsubagent.Definition, 0, len(repository.definitions))
	for _, definition := range repository.definitions {
		if definition.Deleted != includeDeleted && definition.Deleted {
			continue
		}
		items = append(items, domainsubagent.CloneDefinition(definition))
	}
	return items, nil
}

func (repository *Repository) DeleteDefinition(_ context.Context, definitionID string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	definition, ok := repository.definitions[strings.TrimSpace(definitionID)]
	if !ok {
		return subagentport.ErrNotFound
	}
	now := time.Now().UTC()
	definition.Deleted = true
	definition.DeletedAt = &now
	definition.UpdatedAt = now
	repository.definitions[definition.ID] = definition
	return nil
}

func (repository *Repository) SaveTask(_ context.Context, task domainsubagent.Task) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.tasks[task.ID] = domainsubagent.CloneTask(task)
	return nil
}

func (repository *Repository) GetTask(_ context.Context, taskID string) (domainsubagent.Task, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	task, ok := repository.tasks[strings.TrimSpace(taskID)]
	if !ok {
		return domainsubagent.Task{}, subagentport.ErrNotFound
	}
	return domainsubagent.CloneTask(task), nil
}

func (repository *Repository) ListTasks(_ context.Context, parentSessionID string, limit int) ([]domainsubagent.Task, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainsubagent.Task, 0)
	for _, task := range repository.tasks {
		if parentSessionID != "" && task.ParentSessionID != parentSessionID {
			continue
		}
		items = append(items, domainsubagent.CloneTask(task))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (repository *Repository) ListNonTerminalTasks(_ context.Context) ([]domainsubagent.Task, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainsubagent.Task, 0)
	for _, task := range repository.tasks {
		if task.Terminal() {
			continue
		}
		items = append(items, domainsubagent.CloneTask(task))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (repository *Repository) SaveTaskAndRun(_ context.Context, task domainsubagent.Task, run domainsubagent.Run) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.saveTaskAndRunLocked(task, run)
	return nil
}

func (repository *Repository) SaveOwnedTaskAndRun(_ context.Context, task domainsubagent.Task, run domainsubagent.Run, ownerID string, ttl time.Duration) error {
	if ttl <= 0 {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	now := time.Now().UTC()
	repository.mu.Lock()
	defer repository.mu.Unlock()
	lease, exists := repository.leases[task.ID]
	if !exists || lease.runID != run.ID || lease.ownerID != ownerID || !lease.expiresAt.After(now) {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	if current, exists := repository.tasks[task.ID]; exists {
		var err error
		task, err = subagentport.ReconcileOwnedTaskTransition(current, task, run.ID)
		if err != nil {
			return err
		}
	}
	lease.expiresAt = now.Add(ttl)
	repository.leases[task.ID] = lease
	repository.saveTaskAndRunLocked(task, run)
	return nil
}

func (repository *Repository) SaveCanceledTaskAndRun(_ context.Context, task domainsubagent.Task, run domainsubagent.Run) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	current, exists := repository.tasks[task.ID]
	if !exists {
		return subagentport.ErrNotFound
	}
	if current.CurrentRunID != run.ID || (current.Terminal() && current.Status != domainsubagent.TaskStatusCanceled) {
		return subagentport.ErrTaskStateConflict
	}
	repository.saveTaskAndRunLocked(task, run)
	return nil
}

func (repository *Repository) SaveTaskIfVersion(_ context.Context, task domainsubagent.Task, expectedUpdatedAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	current, exists := repository.tasks[task.ID]
	if !exists {
		return subagentport.ErrNotFound
	}
	if !current.UpdatedAt.Equal(expectedUpdatedAt) {
		return subagentport.ErrTaskStateConflict
	}
	repository.tasks[task.ID] = domainsubagent.CloneTask(task)
	return nil
}

func (repository *Repository) SaveOwnedTask(_ context.Context, task domainsubagent.Task, leaseRunID, expectedCurrentRunID, ownerID string, expectedUpdatedAt time.Time, ttl time.Duration) error {
	if ttl <= 0 || strings.TrimSpace(leaseRunID) == "" || strings.TrimSpace(ownerID) == "" {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	lease, exists := repository.leases[task.ID]
	if !exists || lease.runID != leaseRunID || lease.ownerID != ownerID || !lease.expiresAt.After(time.Now().UTC()) {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	current, exists := repository.tasks[task.ID]
	if !exists {
		return subagentport.ErrNotFound
	}
	if current.CurrentRunID != expectedCurrentRunID || !current.UpdatedAt.Equal(expectedUpdatedAt) {
		return subagentport.ErrTaskStateConflict
	}
	lease.expiresAt = time.Now().UTC().Add(ttl)
	repository.leases[task.ID] = lease
	repository.tasks[task.ID] = domainsubagent.CloneTask(task)
	return nil
}

func (repository *Repository) saveTaskAndRunLocked(task domainsubagent.Task, run domainsubagent.Run) {
	repository.tasks[task.ID] = domainsubagent.CloneTask(task)
	items := repository.runs[run.TaskID]
	for index := range items {
		if items[index].ID == run.ID {
			items[index] = domainsubagent.CloneRun(run)
			repository.runs[run.TaskID] = items
			return
		}
	}
	repository.runs[run.TaskID] = append(items, domainsubagent.CloneRun(run))
}

func (repository *Repository) SaveRun(_ context.Context, run domainsubagent.Run) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	items := repository.runs[run.TaskID]
	for index := range items {
		if items[index].ID == run.ID {
			items[index] = domainsubagent.CloneRun(run)
			repository.runs[run.TaskID] = items
			return nil
		}
	}
	repository.runs[run.TaskID] = append(items, domainsubagent.CloneRun(run))
	return nil
}

func (repository *Repository) ListRuns(_ context.Context, taskID string) ([]domainsubagent.Run, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := repository.runs[strings.TrimSpace(taskID)]
	result := make([]domainsubagent.Run, len(items))
	for index, run := range items {
		result[index] = domainsubagent.CloneRun(run)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Sequence < result[j].Sequence })
	return result, nil
}

func (repository *Repository) SaveTaskEvent(_ context.Context, event subagentport.TaskEvent) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for index := range repository.events {
		if repository.events[index].Sequence == event.Sequence {
			repository.events[index] = cloneTaskEvent(event)
			if event.Sequence > repository.eventSequence {
				repository.eventSequence = event.Sequence
			}
			return nil
		}
	}
	if event.Sequence > repository.eventSequence {
		repository.eventSequence = event.Sequence
	}
	if len(repository.events) == 0 || repository.events[len(repository.events)-1].Sequence < event.Sequence {
		repository.events = append(repository.events, cloneTaskEvent(event))
		return nil
	}
	repository.events = append(repository.events, cloneTaskEvent(event))
	sort.Slice(repository.events, func(i, j int) bool { return repository.events[i].Sequence < repository.events[j].Sequence })
	return nil
}

func (repository *Repository) NextTaskEventSequence(_ context.Context) (uint64, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.eventSequence++
	return repository.eventSequence, nil
}

func (repository *Repository) EnsureTaskEventSequence(_ context.Context, atLeast uint64) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if atLeast > repository.eventSequence {
		repository.eventSequence = atLeast
	}
	return nil
}

func (repository *Repository) ListTaskEvents(_ context.Context, parentSessionID string, afterSequence uint64, limit int) ([]subagentport.TaskEvent, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	parentSessionID = strings.TrimSpace(parentSessionID)
	items := make([]subagentport.TaskEvent, 0)
	for _, event := range repository.events {
		if event.Sequence <= afterSequence {
			continue
		}
		if parentSessionID != "" && event.Task.ParentSessionID != parentSessionID {
			continue
		}
		items = append(items, cloneTaskEvent(event))
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items, nil
}

func (repository *Repository) LatestTaskEventSequence(_ context.Context) (uint64, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	if len(repository.events) == 0 {
		return 0, nil
	}
	return repository.events[len(repository.events)-1].Sequence, nil
}

func cloneTaskEvent(event subagentport.TaskEvent) subagentport.TaskEvent {
	event.Task = domainsubagent.CloneTask(event.Task)
	return event
}
