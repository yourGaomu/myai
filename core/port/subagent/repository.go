package subagent

import (
	"context"
	"errors"
	"time"

	domainsubagent "myai/core/domain/subagent"
)

var ErrNotFound = errors.New("subagent record not found")

// ErrExecutionLeaseNotAcquired means another process currently owns the
// execution lease for the same task/run, or the lease has expired before a
// renew/release operation could be completed.
var ErrExecutionLeaseNotAcquired = errors.New("subagent execution lease not acquired")
var ErrPendingMailbox = errors.New("subagent mailbox received a message before completion")
var ErrTaskStateConflict = errors.New("subagent task state changed during transition")

type DefinitionRepository interface {
	SaveDefinition(ctx context.Context, definition domainsubagent.Definition) error
	GetDefinition(ctx context.Context, definitionID string) (domainsubagent.Definition, error)
	ListDefinitions(ctx context.Context, includeDeleted bool) ([]domainsubagent.Definition, error)
	DeleteDefinition(ctx context.Context, definitionID string) error
}

type TaskRepository interface {
	SaveTask(ctx context.Context, task domainsubagent.Task) error
	GetTask(ctx context.Context, taskID string) (domainsubagent.Task, error)
	ListTasks(ctx context.Context, parentSessionID string, limit int) ([]domainsubagent.Task, error)
}

// TaskRunRepository persists one task transition and its current run as a
// single unit. Adapters must not leave only one side of the transition saved.
type TaskRunRepository interface {
	SaveTaskAndRun(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run) error
}

// LeaseGuardedTaskRunRepository commits a task/run transition only while the
// caller still owns its unexpired execution lease. The lease check and state
// write must be atomic in the underlying store.
type LeaseGuardedTaskRunRepository interface {
	SaveOwnedTaskAndRun(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run, ownerID string, ttl time.Duration) error
}

// TaskCancellationRepository commits a user/parent cancellation without
// requiring the caller to own the execution lease. The update must be
// conditional on the task still pointing at the supplied run, so an older
// cancel request cannot overwrite a later follow-up run.
type TaskCancellationRepository interface {
	SaveCanceledTaskAndRun(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run) error
}

// TaskVersionRepository provides optimistic concurrency for task-only writes.
// The expected timestamp is read together with the task and must still match
// when the mutation is committed.
type TaskVersionRepository interface {
	SaveTaskIfVersion(ctx context.Context, task domainsubagent.Task, expectedUpdatedAt time.Time) error
}

// LeaseGuardedTaskRepository serializes a task-only operation with execution
// admission. leaseRunID identifies the short-lived operation lease, while
// expectedCurrentRunID prevents an old operation from overwriting a follow-up.
type LeaseGuardedTaskRepository interface {
	SaveOwnedTask(ctx context.Context, task domainsubagent.Task, leaseRunID, expectedCurrentRunID, ownerID string, expectedUpdatedAt time.Time, ttl time.Duration) error
}

// ReconcileOwnedTaskTransition uses the current mailbox projection while
// committing an execution transition. Input may arrive after the service
// loaded its task snapshot but before the atomic write begins.
func ReconcileOwnedTaskTransition(current, desired domainsubagent.Task, runID string) (domainsubagent.Task, error) {
	if current.ID == "" {
		return desired, nil
	}
	if current.CurrentRunID != "" && current.CurrentRunID != runID && !(current.Terminal() && desired.Status == domainsubagent.TaskStatusQueued) {
		return domainsubagent.Task{}, ErrTaskStateConflict
	}
	if current.Status == domainsubagent.TaskStatusCanceled && desired.Status != domainsubagent.TaskStatusCanceled {
		return domainsubagent.Task{}, ErrTaskStateConflict
	}
	if current.Terminal() && desired.Status != domainsubagent.TaskStatusQueued && current.Status != desired.Status {
		return domainsubagent.Task{}, ErrTaskStateConflict
	}
	desired.Mailbox = domainsubagent.CloneTask(current).Mailbox
	if desired.Status == domainsubagent.TaskStatusSucceeded && desired.HasMailboxMessages() {
		return domainsubagent.Task{}, ErrPendingMailbox
	}
	return desired, nil
}

// InterruptedTaskRepository exposes the non-terminal tasks that need to be
// reconciled after an unclean process shutdown.
type InterruptedTaskRepository interface {
	ListNonTerminalTasks(ctx context.Context) ([]domainsubagent.Task, error)
}

type RunRepository interface {
	SaveRun(ctx context.Context, run domainsubagent.Run) error
	ListRuns(ctx context.Context, taskID string) ([]domainsubagent.Run, error)
}

// AgentExecutionLeaseStore coordinates one active AgentTurn across process
// boundaries. Runtime Manager remains a fast process-local guard; this store
// is the durable ownership boundary used by multiple application instances.
type AgentExecutionLeaseStore interface {
	Acquire(ctx context.Context, taskID, runID, ownerID string, ttl time.Duration) error
	Renew(ctx context.Context, taskID, runID, ownerID string, ttl time.Duration) error
	Release(ctx context.Context, taskID, runID, ownerID string) error
}

// TaskEventRepository stores the ordered task event log used for reconnect
// and replay. When paired with TaskEventSequenceRepository, Sequence is
// allocated atomically by the durable store and is global across instances.
type TaskEventRepository interface {
	SaveTaskEvent(ctx context.Context, event TaskEvent) error
	ListTaskEvents(ctx context.Context, parentSessionID string, afterSequence uint64, limit int) ([]TaskEvent, error)
}

type TaskEventTailRepository interface {
	LatestTaskEventSequence(ctx context.Context) (uint64, error)
}

// TaskEventSequenceRepository allocates a globally unique, monotonically
// increasing event sequence for all application instances sharing a store.
type TaskEventSequenceRepository interface {
	NextTaskEventSequence(ctx context.Context) (uint64, error)
}

type TaskEventSequenceInitializer interface {
	EnsureTaskEventSequence(ctx context.Context, atLeast uint64) error
}
