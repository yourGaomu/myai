package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	agentrunruntime "myai/core/application/agentrun/runtime"
	generationcommand "myai/core/application/chat/generation/command"
	subagentcommand "myai/core/application/subagent/command"
	subagentresult "myai/core/application/subagent/result"
	domainmessage "myai/core/domain/message"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	modelport "myai/core/port/model"
	subagentport "myai/core/port/subagent"
	workspaceport "myai/core/port/workspace"
	"myai/core/session"
)

const (
	defaultTaskListLimit = 50
	cancelWaitTimeout    = 5 * time.Second
	maxMailboxFollowUps  = domainsubagent.MaxMailboxMessages
)

var errMailboxPending = subagentport.ErrPendingMailbox

func (service *Service) Start(ctx context.Context, command subagentcommand.StartTask) (subagentresult.Task, error) {
	if service == nil || service.Registry == nil || service.Tasks == nil || service.Runs == nil || service.Scheduler == nil || service.Sessions == nil || service.Runner == nil || service.IDs == nil {
		return subagentresult.Task{}, errors.New("subagent task service is not configured")
	}
	definitionID := strings.TrimSpace(command.DefinitionID)
	definition, ok := service.Registry.Get(definitionID)
	if !ok || definition.Deleted || !definition.Enabled {
		return subagentresult.Task{}, definitionNotFound(definitionID)
	}
	if modelID := strings.TrimSpace(command.ModelID); modelID != "" {
		definition.ModelID = modelID
	}
	if strings.TrimSpace(definition.ModelID) == "" {
		definition.ModelID = strings.TrimSpace(command.FallbackModelID)
	}
	if service.Models != nil {
		if strings.TrimSpace(definition.ModelID) == "" {
			return subagentresult.Task{}, errors.New("subagent model id is required")
		}
		if !service.Models.HasModel(definition.ModelID) {
			return subagentresult.Task{}, fmt.Errorf("subagent model %q is not registered", definition.ModelID)
		}
	}
	if definition.CapabilityMode != domainsubagent.CapabilityModeReadOnly && definition.IsolationMode == domainworkspace.IsolationModeDirect {
		return subagentresult.Task{}, errors.New("writable subagents require an isolated workspace")
	}
	service.admissionMu.Lock()
	parentTask, err := service.validateTaskParent(ctx, command)
	if err != nil {
		service.admissionMu.Unlock()
		return subagentresult.Task{}, err
	}
	now := service.now()
	workspaceRoot := strings.TrimSpace(command.WorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = strings.TrimSpace(service.DefaultWorkspaceRoot)
	}
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	task := domainsubagent.Task{
		ID: service.IDs.NewID(), ParentSessionID: strings.TrimSpace(command.ParentSessionID),
		ParentTaskID:   strings.TrimSpace(command.ParentTaskID),
		ParentRunID:    strings.TrimSpace(command.ParentRunID),
		PlanID:         strings.TrimSpace(command.PlanID),
		StepID:         strings.TrimSpace(command.StepID),
		ChildSessionID: service.IDs.NewID(), CreatedRequestID: strings.TrimSpace(command.CreatedRequestID),
		AgentPath: strings.TrimSpace(command.AgentPath), AgentNickname: strings.TrimSpace(command.AgentNickname),
		DefinitionID: definition.ID, DefinitionVersion: definition.Version, Definition: definition.Snapshot(),
		Instruction: strings.TrimSpace(command.Instruction), Title: strings.TrimSpace(command.Title),
		Status: domainsubagent.TaskStatusQueued, Unread: false, CreatedAt: now, UpdatedAt: now,
		Workspace: isolatedWorkspaceReference(service.IDs.NewID(), definition.IsolationMode, workspaceRoot),
	}
	if task.Title == "" {
		task.Title = definition.Name
	}
	if task.AgentPath == "" {
		if parentTask.ID != "" {
			task.AgentPath = nestedAgentPath(parentTask, command.TaskName, task.ID)
		} else {
			task.AgentPath = strings.TrimSpace(command.TaskName)
		}
	}
	if task.AgentPath == "" {
		task.AgentPath = task.ID
	}
	if err := task.ValidateNew(); err != nil {
		service.admissionMu.Unlock()
		return subagentresult.Task{}, err
	}
	pathReserved := false
	if service.AgentPaths != nil {
		if err := service.AgentPaths.Reserve(task.AgentPath); err != nil {
			service.admissionMu.Unlock()
			return subagentresult.Task{}, fmt.Errorf("reserve subagent path %q: %w", task.AgentPath, err)
		}
		pathReserved = true
	}
	runtimeRegistered := false
	if service.Runtime != nil {
		if err := service.Runtime.Register(task); err != nil {
			if pathReserved {
				service.AgentPaths.Release(task.AgentPath)
			}
			service.admissionMu.Unlock()
			return subagentresult.Task{}, fmt.Errorf("register subagent runtime: %w", err)
		}
		runtimeRegistered = true
	}
	run := domainsubagent.Run{
		ID: service.IDs.NewID(), TaskID: task.ID, Sequence: 1, Instruction: task.Instruction,
		Status: domainsubagent.RunStatusQueued, CreatedAt: now,
	}
	task.CurrentRunID = run.ID
	if err := service.acquireExecutionLease(ctx, task.ID, run.ID); err != nil {
		if runtimeRegistered {
			_ = service.Runtime.Remove(task.ID)
		}
		if pathReserved {
			service.AgentPaths.Release(task.AgentPath)
		}
		service.admissionMu.Unlock()
		return subagentresult.Task{}, err
	}
	service.mu.Lock()
	if err := service.saveTaskAndRun(ctx, task, run); err != nil {
		service.mu.Unlock()
		service.releaseExecutionLease(task.ID, run.ID)
		if runtimeRegistered {
			_ = service.Runtime.Remove(task.ID)
		}
		if pathReserved {
			service.AgentPaths.Release(task.AgentPath)
		}
		service.admissionMu.Unlock()
		return subagentresult.Task{}, err
	}
	if !service.registerActiveRunLocked(task.ID, run.ID) {
		service.mu.Unlock()
		service.releaseExecutionLease(task.ID, run.ID)
		service.admissionMu.Unlock()
		return subagentresult.Task{}, errors.New("subagent task already has an active execution")
	}
	service.publish(ctx, task)
	service.mu.Unlock()
	service.admissionMu.Unlock()
	return service.scheduleRun(task, run, command.FallbackModelID)
}

func (service *Service) Check(ctx context.Context, command subagentcommand.CheckTask) (subagentresult.Task, error) {
	if service == nil || service.Tasks == nil {
		return subagentresult.Task{}, errors.New("subagent task repository is nil")
	}
	task, err := service.Tasks.GetTask(ctx, strings.TrimSpace(command.TaskID))
	if err != nil {
		return subagentresult.Task{}, err
	}
	if parentID := strings.TrimSpace(command.ParentSessionID); parentID != "" && task.ParentSessionID != parentID {
		return subagentresult.Task{}, subagentport.ErrNotFound
	}
	if !task.Terminal() && task.CreatedRequestID != "" && task.CreatedRequestID == strings.TrimSpace(command.RequestID) {
		return subagentresult.Task{}, errors.New("a subagent task cannot be polled from the request that created it")
	}
	return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
}

// SendMessage queues durable parent input for a non-terminal child. A
// trigger_turn sent to a terminal child is admitted as an idempotent follow-up
// Run on the existing child session.
func (service *Service) SendMessage(ctx context.Context, command subagentcommand.SendMessage) (subagentresult.Task, error) {
	if service == nil || service.Tasks == nil || service.IDs == nil {
		return subagentresult.Task{}, errors.New("subagent mailbox is not configured")
	}
	trigger := command.Trigger
	if trigger == "" {
		trigger = domainsubagent.AgentMessageTriggerQueue
	}
	if trigger == domainsubagent.AgentMessageTriggerSteer {
		return subagentresult.Task{}, errors.New("steer_current_turn is not supported: the runtime has no live model-input steering channel")
	}
	if trigger != domainsubagent.AgentMessageTriggerQueue && trigger != domainsubagent.AgentMessageTriggerTurn {
		return subagentresult.Task{}, fmt.Errorf("unsupported subagent message trigger %q", trigger)
	}
	task, err := service.Tasks.GetTask(ctx, strings.TrimSpace(command.TaskID))
	if err != nil {
		return subagentresult.Task{}, err
	}
	if parentID := strings.TrimSpace(command.ParentSessionID); parentID != "" && task.ParentSessionID != parentID {
		return subagentresult.Task{}, subagentport.ErrNotFound
	}
	messageID := strings.TrimSpace(command.MessageID)
	if messageID == "" {
		messageID = service.IDs.NewID()
	}
	content := strings.TrimSpace(command.Content)
	if content == "" {
		return subagentresult.Task{}, errors.New("subagent message is empty")
	}
	if trigger == domainsubagent.AgentMessageTriggerTurn && service.Runs != nil {
		runs, err := service.Runs.ListRuns(ctx, task.ID)
		if err != nil {
			return subagentresult.Task{}, err
		}
		for _, run := range runs {
			if run.RequestID != messageID {
				continue
			}
			if strings.TrimSpace(run.Instruction) != content {
				return subagentresult.Task{}, errors.New("subagent message id was already used with different content")
			}
			// The Run itself is the durable trigger_turn receipt. This also
			// makes retries idempotent while the new run is still queued.
			latest, loadErr := service.Tasks.GetTask(ctx, task.ID)
			if loadErr != nil {
				return subagentresult.Task{}, loadErr
			}
			return subagentresult.Task{Value: domainsubagent.CloneTask(latest)}, nil
		}
	}
	// A trigger_turn sent to a terminal task starts a new Run on the same
	// child session. Followup performs the durable admission and request-id
	// idempotency checks, so call it without holding service.mu.
	if trigger == domainsubagent.AgentMessageTriggerTurn && task.Terminal() {
		return service.Followup(ctx, subagentcommand.FollowupTask{
			TaskID: task.ID, ParentSessionID: command.ParentSessionID,
			RequestID: messageID, Content: content,
		})
	}
	service.mu.Lock()
	// Refresh after acquiring the process-local mutation lock so an execution
	// completion that raced the initial read cannot make us enqueue onto a
	// terminal snapshot.
	task, err = service.Tasks.GetTask(ctx, task.ID)
	if err != nil {
		service.mu.Unlock()
		return subagentresult.Task{}, err
	}
	if parentID := strings.TrimSpace(command.ParentSessionID); parentID != "" && task.ParentSessionID != parentID {
		service.mu.Unlock()
		return subagentresult.Task{}, subagentport.ErrNotFound
	}
	if trigger == domainsubagent.AgentMessageTriggerTurn && task.Terminal() {
		service.mu.Unlock()
		return service.Followup(ctx, subagentcommand.FollowupTask{
			TaskID: task.ID, ParentSessionID: command.ParentSessionID,
			RequestID: messageID, Content: content,
		})
	}
	if service.AgentMessages != nil {
		repository, ok := service.Tasks.(subagentport.ChildAgentMessageRepository)
		if !ok {
			service.mu.Unlock()
			return subagentresult.Task{}, errors.New("atomic child agent message repository is not configured")
		}
		kind := command.Kind
		if kind == "" {
			kind = domainsubagent.AgentMessageKindInterAgent
		}
		if kind != domainsubagent.AgentMessageKindInterAgent && kind != domainsubagent.AgentMessageKindUserInput {
			service.mu.Unlock()
			return subagentresult.Task{}, fmt.Errorf("subagent message kind %q cannot be queued as child input", kind)
		}
		message := domainsubagent.AgentMessage{
			ID: messageID, SourceTaskID: task.ParentTaskID, AuthorAgentID: task.ParentSessionID,
			RecipientAgentID: task.ChildSessionID, ParentTurnID: task.ParentRunID,
			RootAgentID: service.rootAgentID(ctx, task), Kind: kind, Content: content,
			Trigger: trigger, Status: domainsubagent.AgentMessagePending, CreatedAt: service.now(),
		}
		if err := message.Validate(); err != nil {
			service.mu.Unlock()
			return subagentresult.Task{}, err
		}
		task, err = repository.EnqueueChildAgentMessage(ctx, task.ID, message)
		if err != nil {
			service.mu.Unlock()
			return subagentresult.Task{}, err
		}
		if task.Status == domainsubagent.TaskStatusWaitingSubagents {
			service.signalWaitingTask(task.ID)
		}
		service.publish(ctx, task)
		service.mu.Unlock()
		return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
	}
	for _, queued := range task.Mailbox {
		if queued.ID != messageID {
			continue
		}
		queuedTrigger := queued.Trigger
		if queuedTrigger == "" {
			queuedTrigger = domainsubagent.AgentMessageTriggerQueue
		}
		if strings.TrimSpace(queued.Content) != content || queuedTrigger != trigger {
			service.mu.Unlock()
			return subagentresult.Task{}, errors.New("subagent message id was already used with different request")
		}
		service.mu.Unlock()
		return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
	}
	if err := task.EnqueueMessage(domainsubagent.Message{ID: messageID, Content: content, Trigger: trigger, CreatedAt: service.now()}); err != nil {
		service.mu.Unlock()
		return subagentresult.Task{}, err
	}
	if err := service.Tasks.SaveTask(ctx, task); err != nil {
		service.mu.Unlock()
		return subagentresult.Task{}, err
	}
	if task.Status == domainsubagent.TaskStatusWaitingSubagents {
		service.signalWaitingTask(task.ID)
	}
	service.publish(ctx, task)
	service.mu.Unlock()
	return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
}

func (service *Service) rootAgentID(ctx context.Context, task domainsubagent.Task) string {
	root := task.ParentSessionID
	seen := make(map[string]struct{})
	for parentID := strings.TrimSpace(task.ParentTaskID); parentID != ""; {
		if _, exists := seen[parentID]; exists {
			break
		}
		seen[parentID] = struct{}{}
		parent, err := service.Tasks.GetTask(ctx, parentID)
		if err != nil {
			break
		}
		root = parent.ParentSessionID
		parentID = strings.TrimSpace(parent.ParentTaskID)
	}
	return root
}

// signalWaitingTask wakes a child that is blocked inside wait_agent. The
// channel is closed so all concurrent waiters observe the same mailbox write.
func (service *Service) signalWaitingTask(taskID string) {
	if service == nil || taskID == "" {
		return
	}
	for waiter := range service.waitingWakeups[taskID] {
		if !waiter.signaled {
			close(waiter.wakeup)
			waiter.signaled = true
		}
	}
}

func (service *Service) List(ctx context.Context, command subagentcommand.ListTasks) (subagentresult.Tasks, error) {
	if service == nil || service.Tasks == nil {
		return subagentresult.Tasks{}, errors.New("subagent task repository is nil")
	}
	limit := command.Limit
	if limit <= 0 {
		limit = defaultTaskListLimit
	}
	parentSessionID := strings.TrimSpace(command.ParentSessionID)
	// A parent agent needs the complete task tree, not only its direct children.
	// Recurse through child session IDs so nested spawn_agent calls remain
	// visible to the same parent without changing repository query contracts.
	queue := []string{parentSessionID}
	seenSessions := make(map[string]struct{})
	items := make([]domainsubagent.Task, 0, limit)
	seenTasks := make(map[string]struct{})
	for len(queue) > 0 && len(items) < limit {
		sessionID := queue[0]
		queue = queue[1:]
		if _, seen := seenSessions[sessionID]; seen {
			continue
		}
		seenSessions[sessionID] = struct{}{}
		batch, err := service.Tasks.ListTasks(ctx, sessionID, limit)
		if err != nil {
			return subagentresult.Tasks{}, err
		}
		for _, task := range batch {
			if _, seen := seenTasks[task.ID]; seen {
				continue
			}
			seenTasks[task.ID] = struct{}{}
			items = append(items, task)
			if task.ChildSessionID != "" {
				queue = append(queue, task.ChildSessionID)
			}
			if len(items) >= limit {
				break
			}
		}
	}
	return subagentresult.Tasks{Items: items}, nil
}

func (service *Service) Cancel(ctx context.Context, command subagentcommand.CancelTask) (subagentresult.Task, error) {
	if service == nil || service.Tasks == nil || service.Runs == nil || service.Scheduler == nil {
		return subagentresult.Task{}, errors.New("subagent task service is not configured")
	}
	service.mu.Lock()
	task, err := service.Tasks.GetTask(ctx, strings.TrimSpace(command.TaskID))
	if err != nil {
		service.mu.Unlock()
		return subagentresult.Task{}, err
	}
	if parentID := strings.TrimSpace(command.ParentSessionID); parentID != "" && task.ParentSessionID != parentID {
		service.mu.Unlock()
		return subagentresult.Task{}, subagentport.ErrNotFound
	}
	if task.Terminal() {
		service.mu.Unlock()
		return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
	}
	run, err := service.currentRun(ctx, task)
	if err != nil {
		service.mu.Unlock()
		return subagentresult.Task{}, err
	}
	service.Scheduler.Cancel(task.CurrentRunID)
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		reason = "canceled by user or parent agent"
	}
	now := service.now()
	if err := task.MarkCanceled(reason, now); err != nil {
		service.mu.Unlock()
		return subagentresult.Task{}, err
	}
	run.Status = domainsubagent.RunStatusCanceled
	run.ErrorMessage = reason
	run.CompletedAt = timePtr(now)
	if err := service.saveCanceledTaskAndRun(ctx, task, run); err != nil {
		service.mu.Unlock()
		return subagentresult.Task{}, err
	}
	if service.Runtime != nil {
		_ = service.Runtime.SetStatus(task.ID, domainsubagent.AgentStatusInterrupted)
	}
	service.publish(ctx, task)
	var done <-chan struct{}
	if active := service.activeRuns[task.ID]; active != nil {
		done = active.done
	}
	result := domainsubagent.CloneTask(task)
	service.mu.Unlock()

	if done != nil {
		waitCtx, waitCancel := context.WithTimeout(context.Background(), cancelWaitTimeout)
		select {
		case <-done:
		case <-waitCtx.Done():
		}
		waitCancel()
	}
	return subagentresult.Task{Value: result}, nil
}

func (service *Service) Resume(ctx context.Context, command subagentcommand.ResumeTask) (subagentresult.Resume, error) {
	if service == nil || service.Tasks == nil || service.ParentContinuation == nil {
		return subagentresult.Resume{}, errors.New("subagent parent continuation is not configured")
	}

	service.mu.Lock()
	task, err := service.Tasks.GetTask(ctx, strings.TrimSpace(command.TaskID))
	if err != nil {
		service.mu.Unlock()
		return subagentresult.Resume{}, err
	}
	if parentID := strings.TrimSpace(command.ParentSessionID); parentID != "" && task.ParentSessionID != parentID {
		service.mu.Unlock()
		return subagentresult.Resume{}, subagentport.ErrNotFound
	}
	if task.Status != domainsubagent.TaskStatusSucceeded && task.Status != domainsubagent.TaskStatusFailed {
		service.mu.Unlock()
		return subagentresult.Resume{Task: domainsubagent.CloneTask(task)}, fmt.Errorf("subagent task cannot resume its parent in status %s", task.Status)
	}
	if !task.Unread {
		service.mu.Unlock()
		return subagentresult.Resume{Task: domainsubagent.CloneTask(task)}, errors.New("subagent task result has already been consumed")
	}
	if service.resumeClaims == nil {
		service.resumeClaims = make(map[string]struct{})
	}
	if _, claimed := service.resumeClaims[task.ID]; claimed {
		service.mu.Unlock()
		return subagentresult.Resume{Task: domainsubagent.CloneTask(task)}, errors.New("subagent task result is already being consumed")
	}
	operationID := service.taskOperationID("resume", task.ID)
	releaseMutation, leaseErr := service.taskMutationLease(ctx, task.ID, operationID)
	if leaseErr != nil {
		service.mu.Unlock()
		return subagentresult.Resume{Task: domainsubagent.CloneTask(task)}, leaseErr
	}
	defer releaseMutation()
	current, reloadErr := service.Tasks.GetTask(ctx, task.ID)
	if reloadErr != nil {
		service.mu.Unlock()
		return subagentresult.Resume{Task: domainsubagent.CloneTask(task)}, reloadErr
	}
	if current.CurrentRunID != task.CurrentRunID || !current.UpdatedAt.Equal(task.UpdatedAt) || !current.Unread ||
		(current.Status != domainsubagent.TaskStatusSucceeded && current.Status != domainsubagent.TaskStatusFailed) {
		service.mu.Unlock()
		return subagentresult.Resume{Task: domainsubagent.CloneTask(current)}, subagentport.ErrTaskStateConflict
	}
	task = current
	service.resumeClaims[task.ID] = struct{}{}
	claimed := domainsubagent.CloneTask(task)
	claimed.Unread = false
	service.mu.Unlock()

	var continuation subagentport.ParentContinuationResult
	var continueErr error
	if pending, ok := service.ParentContinuation.(subagentport.PendingParentContinuation); ok {
		if notifyErr := service.ensureParentCompletionQueued(claimed); notifyErr != nil {
			continueErr = notifyErr
		} else {
			continuation, continueErr = pending.ContinuePending(ctx, claimed.ParentSessionID, command.Stream)
		}
	} else {
		continuation, continueErr = service.ParentContinuation.Continue(ctx, subagentport.ParentContinuationRequest{
			Task: claimed, Stream: command.Stream,
		})
	}
	if continueErr != nil {
		restored, restoreErr := service.restoreUnreadResult(claimed.ID, operationID)
		service.mu.Lock()
		delete(service.resumeClaims, claimed.ID)
		service.mu.Unlock()
		return subagentresult.Resume{Task: restored}, errors.Join(continueErr, restoreErr)
	}

	service.mu.Lock()
	delete(service.resumeClaims, claimed.ID)
	current, persistErr := service.Tasks.GetTask(context.Background(), claimed.ID)
	if persistErr == nil && current.Unread {
		expectedRunID, expectedUpdatedAt := current.CurrentRunID, current.UpdatedAt
		current.Unread = false
		current.UpdatedAt = service.now()
		persistErr = service.saveTaskMutation(context.Background(), current, expectedRunID, expectedUpdatedAt, operationID)
		if persistErr == nil {
			service.publish(context.Background(), current)
		}
	}
	service.mu.Unlock()
	if persistErr != nil {
		return subagentresult.Resume{Task: claimed}, persistErr
	}

	return subagentresult.Resume{
		Task: claimed, Content: continuation.Content, Reasoning: continuation.Reasoning, Usage: continuation.Usage,
	}, nil
}

func (service *Service) restoreUnreadResult(taskID, operationID string) (domainsubagent.Task, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	task, err := service.Tasks.GetTask(context.Background(), taskID)
	if err != nil {
		return domainsubagent.Task{}, err
	}
	if task.Unread {
		return domainsubagent.CloneTask(task), nil
	}
	expectedRunID, expectedUpdatedAt := task.CurrentRunID, task.UpdatedAt
	task.Unread = true
	task.UpdatedAt = service.now()
	if err := service.saveTaskMutation(context.Background(), task, expectedRunID, expectedUpdatedAt, operationID); err != nil {
		return domainsubagent.CloneTask(task), err
	}
	service.publish(context.Background(), task)
	return domainsubagent.CloneTask(task), nil
}

func (service *Service) ApplyChanges(ctx context.Context, command subagentcommand.ApplyTaskChanges) (subagentresult.Task, error) {
	if service == nil || service.Tasks == nil || service.Workspaces == nil {
		return subagentresult.Task{}, errors.New("subagent workspace service is not configured")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	task, err := service.Tasks.GetTask(ctx, strings.TrimSpace(command.TaskID))
	if err != nil {
		return subagentresult.Task{}, err
	}
	if parentID := strings.TrimSpace(command.ParentSessionID); parentID != "" && task.ParentSessionID != parentID {
		return subagentresult.Task{}, subagentport.ErrNotFound
	}
	if task.Status != domainsubagent.TaskStatusSucceeded {
		return subagentresult.Task{}, fmt.Errorf("subagent task changes cannot be applied in status %s", task.Status)
	}
	if task.Workspace.Mode == domainworkspace.IsolationModeDirect {
		return subagentresult.Task{}, errors.New("direct subagent tasks do not have isolated changes")
	}
	expectedRunID, expectedUpdatedAt := task.CurrentRunID, task.UpdatedAt
	operationID := service.taskOperationID("apply", task.ID)
	releaseMutation, err := service.taskMutationLease(ctx, task.ID, operationID)
	if err != nil {
		return subagentresult.Task{}, err
	}
	// Re-read after admission. Another instance may have completed or changed
	// the task while this operation was waiting for the mutation lease.
	current, err := service.Tasks.GetTask(ctx, task.ID)
	if err != nil {
		return subagentresult.Task{}, err
	}
	if current.CurrentRunID != expectedRunID || !current.UpdatedAt.Equal(expectedUpdatedAt) || current.Status != domainsubagent.TaskStatusSucceeded {
		return subagentresult.Task{}, subagentport.ErrTaskStateConflict
	}
	task = current
	defer releaseMutation()
	result, applyErr := service.Workspaces.Apply(ctx, workspaceport.ApplyRequest{
		Reference: task.Workspace, TaskID: task.ID, SessionID: task.ParentSessionID,
		RequestID: strings.TrimSpace(command.RequestID), Title: "Apply subagent task: " + task.Title,
	})
	if result.ChangeSet.WorkspaceID != "" {
		task.ChangeSet = domainworkspace.CloneChangeSet(result.ChangeSet)
		if applyErr == nil {
			task.Workspace.SandboxID = ""
		}
		task.UpdatedAt = service.now()
		if saveErr := service.saveTaskMutation(ctx, task, expectedRunID, expectedUpdatedAt, operationID); saveErr != nil {
			return subagentresult.Task{}, errors.Join(applyErr, saveErr)
		}
		service.publish(ctx, task)
	}
	if applyErr != nil {
		return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, applyErr
	}
	return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
}

func (service *Service) DiscardChanges(ctx context.Context, command subagentcommand.DiscardTaskChanges) (subagentresult.Task, error) {
	if service == nil || service.Tasks == nil || service.Workspaces == nil {
		return subagentresult.Task{}, errors.New("subagent workspace service is not configured")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	task, err := service.Tasks.GetTask(ctx, strings.TrimSpace(command.TaskID))
	if err != nil {
		return subagentresult.Task{}, err
	}
	if parentID := strings.TrimSpace(command.ParentSessionID); parentID != "" && task.ParentSessionID != parentID {
		return subagentresult.Task{}, subagentport.ErrNotFound
	}
	if !task.Terminal() {
		return subagentresult.Task{}, errors.New("running subagent task changes cannot be discarded")
	}
	if task.Workspace.Mode == domainworkspace.IsolationModeDirect {
		return subagentresult.Task{}, errors.New("direct subagent tasks do not have isolated changes")
	}
	expectedRunID, expectedUpdatedAt := task.CurrentRunID, task.UpdatedAt
	operationID := service.taskOperationID("discard", task.ID)
	releaseMutation, err := service.taskMutationLease(ctx, task.ID, operationID)
	if err != nil {
		return subagentresult.Task{}, err
	}
	current, err := service.Tasks.GetTask(ctx, task.ID)
	if err != nil {
		return subagentresult.Task{}, err
	}
	if current.CurrentRunID != expectedRunID || !current.UpdatedAt.Equal(expectedUpdatedAt) || !current.Terminal() {
		return subagentresult.Task{}, subagentport.ErrTaskStateConflict
	}
	task = current
	defer releaseMutation()
	result, err := service.Workspaces.Discard(ctx, workspaceport.DiscardRequest{
		Reference: task.Workspace, DiscardedAt: service.now(), RequestID: operationID,
	})
	if err != nil {
		return subagentresult.Task{}, err
	}
	task.ChangeSet = domainworkspace.CloneChangeSet(result.ChangeSet)
	task.Workspace.SandboxID = ""
	task.UpdatedAt = service.now()
	if err := service.saveTaskMutation(ctx, task, expectedRunID, expectedUpdatedAt, operationID); err != nil {
		return subagentresult.Task{}, err
	}
	service.publish(ctx, task)
	return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
}

func (service *Service) execute(ctx context.Context, taskID string, runID string, fallbackModelID string) {
	defer service.releaseExecutionLease(taskID, runID)
	defer service.completeActiveRun(taskID, runID)
	leaseContext, leaseCancel := context.WithCancel(ctx)
	var leaseLost atomic.Bool
	leaseDone := service.watchExecutionLease(leaseContext, taskID, runID, leaseCancel, &leaseLost)
	defer func() {
		leaseCancel()
		<-leaseDone
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("subagent execution panicked: %v", recovered)
			if !leaseLost.Load() {
				if err := service.finishPanic(taskID, runID, panicErr); err != nil {
					panicErr = errors.Join(panicErr, err)
				}
			}
			service.reportError(panicErr)
		}
	}()
	initialLeaseCheck, initialLeaseCancel := context.WithTimeout(leaseContext, service.leaseRenewTimeout())
	err := service.renewExecutionLease(initialLeaseCheck, taskID, runID)
	initialLeaseCancel()
	if err != nil {
		service.reportError(fmt.Errorf("start subagent task %s without execution lease: %w", taskID, err))
		return
	}
	task, run, ok, err := service.beginExecution(leaseContext, taskID, runID)
	if err != nil {
		service.reportError(fmt.Errorf("start subagent task %s execution: %w", taskID, err))
		return
	}
	if !ok {
		return
	}
	timeout := time.Duration(task.Definition.TimeoutSeconds) * time.Second
	runContext, cancel := context.WithTimeout(leaseContext, timeout)
	defer cancel()
	task, err = service.prepareWorkspace(runContext, task)

	if err == nil {
		_, err = service.Sessions.Create(runContext, subagentport.ChildSessionRequest{
			SessionID: task.ChildSessionID, ParentSessionID: task.ParentSessionID, ParentTaskID: task.ID,
			Definition: task.Definition, FallbackModelID: fallbackModelID, WorkspaceRoot: task.Workspace.Root,
			WorkspaceSandboxID: task.Workspace.SandboxID,
		})
	}
	var response subagentport.AgentRunResult
	if err == nil {
		metadata := agentrunruntime.Metadata{ParentRunID: task.ParentRunID, TaskID: task.ID, PlanID: task.PlanID, StepID: task.StepID}
		runContext = agentrunruntime.WithMetadata(runContext, metadata)
		var injected []domainsubagent.Message
		defer func() {
			for _, message := range injected {
				if releaseErr := service.releaseMailboxMessage(task.ID, message.ID, err); releaseErr != nil {
					service.reportError(fmt.Errorf("release injected subagent mailbox message %s: %w", message.ID, releaseErr))
				}
			}
		}()
		runContext = generationcommand.WithAfterToolRound(runContext, func(hookCtx context.Context, current *session.Session) error {
			claimed, hookErr := service.injectMailboxBetweenToolRounds(hookCtx, task.ID, current)
			injected = append(injected, claimed...)
			return hookErr
		})
		response, err = service.Runner.Run(runContext, subagentport.AgentRunRequest{
			SessionID: task.ChildSessionID, Instruction: run.Instruction, Title: task.Title,
			Stream: service.taskStream(runContext, task.ID, run.ID),
		})
		if err == nil {
			if strings.TrimSpace(response.Content) == "" {
				err = errors.New("subagent returned an empty result")
			} else {
				for len(injected) > 0 {
					if ackErr := service.acknowledgeMailboxMessage(task.ID, injected[0].ID); ackErr != nil {
						err = ackErr
						break
					}
					injected = injected[1:]
				}
			}
		}
		for err == nil {
			if leaseLost.Load() {
				return
			}
			response, err = service.runMailboxFollowUps(runContext, task, run, response)
			if err == nil && strings.TrimSpace(response.Content) == "" {
				err = errors.New("subagent returned an empty result")
			}
			if err != nil {
				break
			}
			var changes domainworkspace.ChangeSet
			changes, err = service.collectWorkspaceChanges(runContext, task)
			if err != nil {
				break
			}
			finishErr := service.finishSuccess(task, run, response, changes)
			if errors.Is(finishErr, errMailboxPending) {
				continue
			}
			if finishErr != nil {
				service.reportError(fmt.Errorf("finish subagent task %s successfully: %w", task.ID, finishErr))
			}
			return
		}
	}
	if leaseLost.Load() {
		return
	}
	if finishErr := service.finishError(task, run, runContext, err, response.Usage); finishErr != nil {
		service.reportError(fmt.Errorf("finish subagent task %s with error: %w", task.ID, finishErr))
	}
}

func (service *Service) injectMailboxBetweenToolRounds(_ context.Context, taskID string, current *session.Session) ([]domainsubagent.Message, error) {
	if service == nil || current == nil || strings.TrimSpace(taskID) == "" {
		return nil, nil
	}
	claimed := make([]domainsubagent.Message, 0)
	for turn := 0; turn < maxMailboxFollowUps; turn++ {
		message, ok, err := service.claimMailboxMessage(taskID)
		if err != nil {
			return claimed, err
		}
		if !ok {
			return claimed, nil
		}
		claimed = append(claimed, message)
		alreadyInSession := false
		for _, existing := range current.Messages {
			if existing.ID == message.ID {
				alreadyInSession = true
				break
			}
		}
		if !alreadyInSession {
			entry := domainmessage.Text(domainmessage.RoleUser, message.Content)
			entry.ID = message.ID
			current.AppendMessage(entry)
		}
	}
	return claimed, nil
}

func (service *Service) runMailboxFollowUps(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run, response subagentport.AgentRunResult) (subagentport.AgentRunResult, error) {
	for turn := 0; turn < maxMailboxFollowUps; turn++ {
		message, ok, err := service.claimMailboxMessage(task.ID)
		if err != nil || !ok {
			return response, err
		}
		followUp, runErr := service.deliverMailboxMessage(ctx, task, run, message)
		if runErr != nil {
			return response, runErr
		}
		response.Content = strings.TrimSpace(response.Content + "\n\n" + followUp.Content)
		response.Reasoning = strings.TrimSpace(response.Reasoning + "\n\n" + followUp.Reasoning)
		response.Usage = response.Usage.Add(followUp.Usage)
	}
	remaining, err := service.mailboxHasMessages(task.ID)
	if err != nil || !remaining {
		return response, err
	}
	return response, errors.New("subagent mailbox follow-up limit reached")
}

func (service *Service) deliverMailboxMessage(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run, message domainsubagent.Message) (response subagentport.AgentRunResult, err error) {
	acknowledged := false
	defer func() {
		if acknowledged {
			return
		}
		if releaseErr := service.releaseMailboxMessage(task.ID, message.ID, err); releaseErr != nil {
			service.reportError(fmt.Errorf("release subagent mailbox message %s: %w", message.ID, releaseErr))
			if err != nil {
				err = errors.Join(err, releaseErr)
			}
		}
	}()
	response, err = service.Runner.Run(ctx, subagentport.AgentRunRequest{
		SessionID: task.ChildSessionID, Instruction: message.Content, Title: task.Title,
		Stream: service.taskStream(ctx, task.ID, run.ID),
	})
	if err != nil {
		return subagentport.AgentRunResult{}, err
	}
	if strings.TrimSpace(response.Content) == "" {
		err = errors.New("subagent follow-up returned an empty result")
		return subagentport.AgentRunResult{}, err
	}
	if err = service.acknowledgeMailboxMessage(task.ID, message.ID); err != nil {
		return subagentport.AgentRunResult{}, err
	}
	acknowledged = true
	return response, nil
}

func (service *Service) claimMailboxMessage(taskID string) (domainsubagent.Message, bool, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.AgentMessages != nil {
		repository, ok := service.Tasks.(subagentport.ChildAgentMessageRepository)
		if !ok {
			return domainsubagent.Message{}, false, errors.New("atomic child agent message repository is not configured")
		}
		message, claimed, err := repository.ClaimChildMailboxMessage(context.Background(), taskID, service.now())
		if err != nil || !claimed {
			return message, claimed, err
		}
		if current, loadErr := service.Tasks.GetTask(context.Background(), taskID); loadErr == nil {
			service.publish(context.Background(), current)
		}
		return message, true, nil
	}
	task, err := service.Tasks.GetTask(context.Background(), taskID)
	if err != nil {
		return domainsubagent.Message{}, false, err
	}
	message, ok := task.ClaimMailboxMessage(service.now())
	if !ok {
		return domainsubagent.Message{}, false, nil
	}
	if err := service.Tasks.SaveTask(context.Background(), task); err != nil {
		return domainsubagent.Message{}, false, err
	}
	service.publish(context.Background(), task)
	return message, true, nil
}

func (service *Service) acknowledgeMailboxMessage(taskID string, messageID string) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.AgentMessages != nil {
		repository, ok := service.Tasks.(subagentport.ChildAgentMessageRepository)
		if !ok {
			return errors.New("atomic child agent message repository is not configured")
		}
		err := repository.AcknowledgeChildAgentMessage(context.Background(), taskID, messageID, service.now())
		if err == nil {
			if current, loadErr := service.Tasks.GetTask(context.Background(), taskID); loadErr == nil {
				service.publish(context.Background(), current)
			}
			return nil
		}
		if !errors.Is(err, subagentport.ErrNotFound) {
			return err
		}
	}
	task, err := service.Tasks.GetTask(context.Background(), taskID)
	if err != nil {
		return err
	}
	if !task.AcknowledgeMailboxMessage(messageID, service.now()) {
		return fmt.Errorf("claimed subagent mailbox message not found: %s", messageID)
	}
	if err := service.Tasks.SaveTask(context.Background(), task); err != nil {
		return err
	}
	service.publish(context.Background(), task)
	return nil
}

func (service *Service) releaseMailboxMessage(taskID string, messageID string, deliveryErr error) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.AgentMessages != nil {
		repository, ok := service.Tasks.(subagentport.ChildAgentMessageRepository)
		if !ok {
			return errors.New("atomic child agent message repository is not configured")
		}
		reason := ""
		if deliveryErr != nil {
			reason = deliveryErr.Error()
		}
		if err := repository.ReleaseChildMailboxMessage(context.Background(), taskID, messageID, reason, service.now()); err != nil {
			return err
		}
		if current, loadErr := service.Tasks.GetTask(context.Background(), taskID); loadErr == nil {
			service.publish(context.Background(), current)
		}
		return nil
	}
	task, err := service.Tasks.GetTask(context.Background(), taskID)
	if err != nil {
		return err
	}
	if !task.ReleaseMailboxMessage(messageID, deliveryErr, service.now()) {
		return nil
	}
	if err := service.Tasks.SaveTask(context.Background(), task); err != nil {
		return err
	}
	service.publish(context.Background(), task)
	return nil
}

func (service *Service) mailboxHasMessages(taskID string) (bool, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	task, err := service.Tasks.GetTask(context.Background(), taskID)
	if err != nil {
		return false, err
	}
	return task.HasMailboxMessages(), nil
}

func (service *Service) completeActiveRun(taskID, runID string) {
	if service == nil || taskID == "" {
		return
	}
	service.mu.Lock()
	var sessionID string
	if service.Tasks != nil {
		if task, err := service.Tasks.GetTask(context.Background(), taskID); err == nil && task.Terminal() {
			sessionID = strings.TrimSpace(task.ChildSessionID)
		}
	}
	released := service.completeActiveRunLocked(taskID, runID)
	if released && sessionID != "" && service.activeRuns[taskID] == nil {
		service.unloadChildSessionLocked(context.Background(), taskID, sessionID)
	}
	service.mu.Unlock()
}

// unloadChildSessionLocked must be called while service.mu is held. Keeping
// the unload under the same admission lock prevents an old run's cleanup from
// racing a newly admitted follow-up run for the same child session.
func (service *Service) unloadChildSessionLocked(ctx context.Context, taskID, sessionID string) {
	if service == nil || service.Sessions == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	lifecycle, ok := service.Sessions.(subagentport.ChildSessionLifecycle)
	if !ok {
		return
	}
	if err := lifecycle.Unload(ctx, strings.TrimSpace(sessionID)); err != nil {
		service.reportError(fmt.Errorf("unload subagent session %s: %w", sessionID, err))
		return
	}
	if service.Runtime != nil {
		_ = service.Runtime.MarkUnloaded(strings.TrimSpace(taskID))
	}
}

func (service *Service) completeActiveRunLocked(taskID, runID string) bool {
	released := false
	if active := service.activeRuns[taskID]; active != nil && active.runID == runID {
		delete(service.activeRuns, taskID)
		close(active.done)
		released = true
	}
	if service.Runtime != nil {
		_ = service.Runtime.ReleaseTurn(taskID, runID)
	}
	return released
}

func (service *Service) registerActiveRun(taskID, runID string) bool {
	if service == nil || taskID == "" {
		return false
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.registerActiveRunLocked(taskID, runID)
}

func (service *Service) registerActiveRunLocked(taskID, runID string) bool {
	if service.activeRuns == nil {
		service.activeRuns = make(map[string]*activeExecution)
	}
	if _, exists := service.activeRuns[taskID]; exists {
		return false
	}
	if service.Runtime != nil {
		if err := service.Runtime.ReserveTurn(taskID, runID); err != nil {
			return false
		}
	}
	service.activeRuns[taskID] = &activeExecution{runID: runID, done: make(chan struct{})}
	return true
}

func (service *Service) taskIsActive(taskID string) bool {
	if service == nil || taskID == "" {
		return false
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	_, active := service.activeRuns[taskID]
	return active
}

func isolatedWorkspaceReference(id string, mode domainworkspace.IsolationMode, sourceRoot string) domainworkspace.Reference {
	reference := domainworkspace.Reference{ID: id, Mode: mode, Root: sourceRoot}
	if mode != domainworkspace.IsolationModeDirect {
		reference.SourceRoot = sourceRoot
	}
	return reference
}

func workspaceSourceRoot(task domainsubagent.Task) string {
	if source := strings.TrimSpace(task.Workspace.SourceRoot); source != "" {
		return source
	}
	return strings.TrimSpace(task.Workspace.Root)
}

func (service *Service) prepareWorkspace(ctx context.Context, task domainsubagent.Task) (domainsubagent.Task, error) {
	if task.Workspace.Mode == domainworkspace.IsolationModeDirect {
		return task, nil
	}
	if service.Workspaces == nil {
		return task, errors.New("isolated workspace manager is not configured")
	}
	if reused, ok, err := service.reuseOpenSandbox(ctx, task); err != nil || ok {
		return reused, err
	}
	prepared, err := service.Workspaces.Prepare(ctx, workspaceport.PrepareRequest{
		WorkspaceID: task.Workspace.ID, Mode: task.Workspace.Mode, SourceRoot: workspaceSourceRoot(task),
		TaskID: task.ID, SessionID: task.ParentSessionID,
	})
	if err != nil {
		return task, err
	}
	if prepared.Reference.SourceRoot == "" {
		prepared.Reference.SourceRoot = workspaceSourceRoot(task)
	}
	service.mu.Lock()
	current, err := service.Tasks.GetTask(context.Background(), task.ID)
	if err == nil && (current.CurrentRunID != task.CurrentRunID || current.Terminal()) {
		err = context.Canceled
	}
	if err == nil {
		current.Workspace = prepared.Reference
		err = service.Tasks.SaveTask(context.Background(), current)
	}
	service.mu.Unlock()
	if err != nil {
		_, _ = service.Workspaces.Discard(context.Background(), workspaceport.DiscardRequest{Reference: prepared.Reference, DiscardedAt: service.now()})
		return task, err
	}
	return current, nil
}

func (service *Service) reuseOpenSandbox(ctx context.Context, task domainsubagent.Task) (domainsubagent.Task, bool, error) {
	if task.Workspace.Mode != domainworkspace.IsolationModeOpenSandbox || strings.TrimSpace(task.Workspace.SandboxID) == "" {
		return task, false, nil
	}
	collected, err := service.Workspaces.Collect(ctx, workspaceport.CollectRequest{Reference: task.Workspace})
	if err != nil {
		return task, false, nil
	}
	switch collected.ChangeSet.Status {
	case domainworkspace.ChangeSetStatusPending, domainworkspace.ChangeSetStatusConflict, domainworkspace.ChangeSetStatusNone, "":
		return task, true, nil
	default:
		return task, false, nil
	}
}

func (service *Service) collectWorkspaceChanges(ctx context.Context, task domainsubagent.Task) (domainworkspace.ChangeSet, error) {
	if task.Workspace.Mode == domainworkspace.IsolationModeDirect {
		return domainworkspace.ChangeSet{Status: domainworkspace.ChangeSetStatusNone}, nil
	}
	collected, err := service.Workspaces.Collect(ctx, workspaceport.CollectRequest{Reference: task.Workspace})
	if err != nil {
		return domainworkspace.ChangeSet{}, err
	}
	return collected.ChangeSet, nil
}

func (service *Service) discardFailedWorkspace(task domainsubagent.Task) {
	if service.Workspaces == nil || task.Workspace.Mode == domainworkspace.IsolationModeDirect || task.Workspace.Root == "" {
		return
	}
	_, _ = service.Workspaces.Discard(context.Background(), workspaceport.DiscardRequest{
		Reference: task.Workspace, DiscardedAt: service.now(),
	})
}

func (service *Service) beginExecution(ctx context.Context, taskID string, runID string) (domainsubagent.Task, domainsubagent.Run, bool, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	task, err := service.Tasks.GetTask(context.Background(), taskID)
	if err != nil {
		return domainsubagent.Task{}, domainsubagent.Run{}, false, err
	}
	if task.Terminal() || task.CurrentRunID != runID {
		return domainsubagent.Task{}, domainsubagent.Run{}, false, nil
	}
	runs, err := service.Runs.ListRuns(context.Background(), taskID)
	if err != nil {
		return domainsubagent.Task{}, domainsubagent.Run{}, false, err
	}
	var run domainsubagent.Run
	for _, item := range runs {
		if item.ID == runID {
			run = item
			break
		}
	}
	if run.ID == "" {
		return domainsubagent.Task{}, domainsubagent.Run{}, false, errors.New("subagent current run not found: " + runID)
	}
	now := service.now()
	if err := task.MarkRunning(now); err != nil {
		return domainsubagent.Task{}, domainsubagent.Run{}, false, err
	}
	run.Status = domainsubagent.RunStatusRunning
	run.StartedAt = timePtr(now)
	if err := service.saveTaskAndRun(context.Background(), task, run); err != nil {
		return domainsubagent.Task{}, domainsubagent.Run{}, false, err
	}
	if service.Runtime != nil {
		_ = service.Runtime.MarkLoaded(task.ID)
		_ = service.Runtime.SetStatus(task.ID, domainsubagent.AgentStatusRunning)
	}
	service.publish(ctx, task)
	service.publishRuntimeEvent(ctx, task.ID, run.ID, subagentport.TaskEventKindStarted, "", "", "", "running", "", "", false)
	return task, run, true, nil
}

func (service *Service) finishSuccess(task domainsubagent.Task, run domainsubagent.Run, response subagentport.AgentRunResult, changes domainworkspace.ChangeSet) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	current, err := service.Tasks.GetTask(context.Background(), task.ID)
	if err != nil {
		return err
	}
	if current.CurrentRunID != run.ID {
		return nil
	}
	if current.Terminal() {
		if current.Status == domainsubagent.TaskStatusCanceled || current.Status == domainsubagent.TaskStatusFailed {
			if err := service.renewExecutionLease(context.Background(), task.ID, run.ID); err == nil {
				service.discardFailedWorkspace(task)
			}
		}
		return nil
	}
	if current.HasMailboxMessages() {
		return errMailboxPending
	}
	now := service.now()
	if err := current.MarkSucceeded(response.Content, response.Reasoning, now); err != nil {
		return err
	}
	current.Workspace = task.Workspace
	current.ChangeSet = domainworkspace.CloneChangeSet(changes)
	current.Usage = current.Usage.Add(response.Usage)
	run.Status = domainsubagent.RunStatusSucceeded
	run.Result = response.Content
	run.Usage = response.Usage
	run.CompletedAt = timePtr(now)
	if err := service.saveTaskAndRun(context.Background(), current, run); err != nil {
		return err
	}
	if service.Runtime != nil {
		_ = service.Runtime.SetStatus(current.ID, domainsubagent.AgentStatusCompleted)
	}
	service.publish(context.Background(), current)
	service.notifyParentCompletionLocked(current)
	return nil
}

func (service *Service) finishError(task domainsubagent.Task, run domainsubagent.Run, runContext context.Context, runErr error, usage modelport.TokenUsage) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	current, err := service.Tasks.GetTask(context.Background(), task.ID)
	if err != nil {
		return err
	}
	if current.CurrentRunID != run.ID {
		return nil
	}
	if current.Terminal() {
		if current.Status == domainsubagent.TaskStatusCanceled || current.Status == domainsubagent.TaskStatusFailed {
			if err := service.renewExecutionLease(context.Background(), task.ID, run.ID); err == nil {
				service.discardFailedWorkspace(task)
			}
		}
		return nil
	}
	now := service.now()
	message := "subagent execution failed"
	if runErr != nil {
		message = runErr.Error()
	}
	if errors.Is(runContext.Err(), context.Canceled) {
		if err := current.MarkCanceled(message, now); err != nil {
			return err
		}
		run.Status = domainsubagent.RunStatusCanceled
	} else {
		if errors.Is(runContext.Err(), context.DeadlineExceeded) {
			message = fmt.Sprintf("subagent timed out after %d seconds", task.Definition.TimeoutSeconds)
		}
		if err := current.MarkFailed(message, now); err != nil {
			return err
		}
		run.Status = domainsubagent.RunStatusFailed
	}
	run.ErrorMessage = message
	run.Usage = usage
	current.Usage = current.Usage.Add(usage)
	run.CompletedAt = timePtr(now)
	if err := service.saveTaskAndRun(context.Background(), current, run); err != nil {
		return err
	}
	service.discardFailedWorkspace(task)
	if service.Runtime != nil {
		status := domainsubagent.AgentStatusFailed
		if current.Status == domainsubagent.TaskStatusCanceled {
			status = domainsubagent.AgentStatusInterrupted
		}
		_ = service.Runtime.SetStatus(current.ID, status)
	}
	service.publish(context.Background(), current)
	if current.Status != domainsubagent.TaskStatusCanceled {
		service.notifyParentCompletionLocked(current)
	}
	return nil
}

func (service *Service) publish(ctx context.Context, task domainsubagent.Task) {
	if service != nil && service.Events != nil {
		service.Events.TaskUpdated(ctx, domainsubagent.CloneTask(task))
	}
}

func timePtr(value time.Time) *time.Time {
	copy := value
	return &copy
}
