package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	agentrunruntime "myai/core/application/agentrun/runtime"
	subagentcommand "myai/core/application/subagent/command"
	subagentresult "myai/core/application/subagent/result"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	subagentport "myai/core/port/subagent"
	workspaceport "myai/core/port/workspace"
)

const (
	defaultTaskListLimit = 50
	cancelWaitTimeout    = 5 * time.Second
)

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
		Workspace: domainworkspace.Reference{ID: service.IDs.NewID(), Mode: definition.IsolationMode, Root: workspaceRoot},
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
	run := domainsubagent.Run{
		ID: service.IDs.NewID(), TaskID: task.ID, Sequence: 1, Instruction: task.Instruction,
		Status: domainsubagent.RunStatusQueued, CreatedAt: now,
	}
	task.CurrentRunID = run.ID
	if err := service.saveTaskAndRun(ctx, task, run); err != nil {
		service.admissionMu.Unlock()
		return subagentresult.Task{}, err
	}
	service.admissionMu.Unlock()
	service.mu.Lock()
	if service.activeRuns == nil {
		service.activeRuns = make(map[string]chan struct{})
	}
	service.activeRuns[task.ID] = make(chan struct{})
	service.mu.Unlock()
	if err := service.Scheduler.Submit(task.ID, func(runContext context.Context) {
		service.execute(runContext, task.ID, run.ID, command.FallbackModelID)
	}); err != nil {
		now := service.now()
		message := "schedule subagent task: " + err.Error()
		if transitionErr := task.MarkFailed(message, now); transitionErr != nil {
			service.completeActiveRun(task.ID)
			return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, errors.Join(err, transitionErr)
		}
		run.Status = domainsubagent.RunStatusFailed
		run.ErrorMessage = message
		run.CompletedAt = timePtr(now)
		if saveErr := service.saveTaskAndRun(context.Background(), task, run); saveErr != nil {
			service.completeActiveRun(task.ID)
			return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, errors.Join(err, saveErr)
		}
		service.publish(ctx, task)
		service.completeActiveRun(task.ID)
		return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, err
	}
	service.publish(ctx, task)
	return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
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
	service.Scheduler.Cancel(task.ID)
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
	if err := service.saveTaskAndRun(ctx, task, run); err != nil {
		service.mu.Unlock()
		return subagentresult.Task{}, err
	}
	service.publish(ctx, task)
	done := service.activeRuns[task.ID]
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
	service.resumeClaims[task.ID] = struct{}{}
	claimed := domainsubagent.CloneTask(task)
	claimed.Unread = false
	service.mu.Unlock()

	continuation, continueErr := service.ParentContinuation.Continue(ctx, subagentport.ParentContinuationRequest{
		Task: claimed, Stream: command.Stream,
	})
	if continueErr != nil {
		service.mu.Lock()
		delete(service.resumeClaims, claimed.ID)
		service.mu.Unlock()
		restored, restoreErr := service.restoreUnreadResult(claimed.ID)
		return subagentresult.Resume{Task: restored}, errors.Join(continueErr, restoreErr)
	}

	service.mu.Lock()
	delete(service.resumeClaims, claimed.ID)
	current, persistErr := service.Tasks.GetTask(context.Background(), claimed.ID)
	if persistErr == nil && current.Unread {
		current.Unread = false
		current.UpdatedAt = service.now()
		persistErr = service.Tasks.SaveTask(context.Background(), current)
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

func (service *Service) restoreUnreadResult(taskID string) (domainsubagent.Task, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	task, err := service.Tasks.GetTask(context.Background(), taskID)
	if err != nil {
		return domainsubagent.Task{}, err
	}
	if task.Unread {
		return domainsubagent.CloneTask(task), nil
	}
	task.Unread = true
	task.UpdatedAt = service.now()
	if err := service.Tasks.SaveTask(context.Background(), task); err != nil {
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
	result, applyErr := service.Workspaces.Apply(ctx, workspaceport.ApplyRequest{
		Reference: task.Workspace, TaskID: task.ID, SessionID: task.ParentSessionID,
		RequestID: strings.TrimSpace(command.RequestID), Title: "Apply subagent task: " + task.Title,
	})
	if result.ChangeSet.WorkspaceID != "" {
		task.ChangeSet = domainworkspace.CloneChangeSet(result.ChangeSet)
		task.UpdatedAt = service.now()
		if saveErr := service.Tasks.SaveTask(ctx, task); saveErr != nil {
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
	result, err := service.Workspaces.Discard(ctx, workspaceport.DiscardRequest{
		Reference: task.Workspace, DiscardedAt: service.now(),
	})
	if err != nil {
		return subagentresult.Task{}, err
	}
	task.ChangeSet = domainworkspace.CloneChangeSet(result.ChangeSet)
	task.UpdatedAt = service.now()
	if err := service.Tasks.SaveTask(ctx, task); err != nil {
		return subagentresult.Task{}, err
	}
	service.publish(ctx, task)
	return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
}

func (service *Service) execute(ctx context.Context, taskID string, runID string, fallbackModelID string) {
	defer service.completeActiveRun(taskID)
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("subagent execution panicked: %v", recovered)
			if err := service.finishPanic(taskID, runID, panicErr); err != nil {
				panicErr = errors.Join(panicErr, err)
			}
			service.reportError(panicErr)
		}
	}()
	task, run, ok, err := service.beginExecution(ctx, taskID, runID)
	if err != nil {
		service.reportError(fmt.Errorf("start subagent task %s execution: %w", taskID, err))
		return
	}
	if !ok {
		return
	}
	timeout := time.Duration(task.Definition.TimeoutSeconds) * time.Second
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	task, err = service.prepareWorkspace(runContext, task)

	if err == nil {
		_, err = service.Sessions.Create(runContext, subagentport.ChildSessionRequest{
			SessionID: task.ChildSessionID, ParentSessionID: task.ParentSessionID, ParentTaskID: task.ID,
			Definition: task.Definition, FallbackModelID: fallbackModelID, WorkspaceRoot: task.Workspace.Root,
			WorkspaceSandboxID: task.Workspace.SandboxID,
		})
	}
	if err == nil {
		var response subagentport.AgentRunResult
		metadata := agentrunruntime.Metadata{ParentRunID: task.ParentRunID, TaskID: task.ID, PlanID: task.PlanID, StepID: task.StepID}
		runContext = agentrunruntime.WithMetadata(runContext, metadata)
		response, err = service.Runner.Run(runContext, subagentport.AgentRunRequest{
			SessionID: task.ChildSessionID, Instruction: task.Instruction, Title: task.Title,
			Stream: service.taskStream(runContext, task.ID, run.ID),
		})
		if err == nil && strings.TrimSpace(response.Content) == "" {
			err = errors.New("subagent returned an empty result")
		}
		if err == nil {
			var changes domainworkspace.ChangeSet
			changes, err = service.collectWorkspaceChanges(runContext, task)
			if err == nil {
				if finishErr := service.finishSuccess(task, run, response, changes); finishErr != nil {
					service.reportError(fmt.Errorf("finish subagent task %s successfully: %w", task.ID, finishErr))
				}
				return
			}
		}
	}
	service.discardFailedWorkspace(task)
	if finishErr := service.finishError(task, run, runContext, err); finishErr != nil {
		service.reportError(fmt.Errorf("finish subagent task %s with error: %w", task.ID, finishErr))
	}
}

func (service *Service) completeActiveRun(taskID string) {
	if service == nil || taskID == "" {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if done, ok := service.activeRuns[taskID]; ok {
		delete(service.activeRuns, taskID)
		close(done)
	}
}

func (service *Service) prepareWorkspace(ctx context.Context, task domainsubagent.Task) (domainsubagent.Task, error) {
	if task.Workspace.Mode == domainworkspace.IsolationModeDirect {
		return task, nil
	}
	if service.Workspaces == nil {
		return task, errors.New("isolated workspace manager is not configured")
	}
	prepared, err := service.Workspaces.Prepare(ctx, workspaceport.PrepareRequest{
		WorkspaceID: task.Workspace.ID, Mode: task.Workspace.Mode, SourceRoot: task.Workspace.Root,
		TaskID: task.ID, SessionID: task.ParentSessionID,
	})
	if err != nil {
		return task, err
	}
	original := task
	task.Workspace = prepared.Reference
	if err := service.Tasks.SaveTask(context.Background(), task); err != nil {
		_, _ = service.Workspaces.Discard(context.Background(), workspaceport.DiscardRequest{Reference: prepared.Reference, DiscardedAt: service.now()})
		return original, err
	}
	return task, nil
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
	if task.Terminal() {
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
	if current.Terminal() {
		if current.Status == domainsubagent.TaskStatusCanceled || current.Status == domainsubagent.TaskStatusFailed {
			service.discardFailedWorkspace(task)
		}
		return nil
	}
	now := service.now()
	if err := current.MarkSucceeded(response.Content, response.Reasoning, now); err != nil {
		return err
	}
	current.Workspace = task.Workspace
	current.ChangeSet = domainworkspace.CloneChangeSet(changes)
	run.Status = domainsubagent.RunStatusSucceeded
	run.Result = response.Content
	run.CompletedAt = timePtr(now)
	if err := service.saveTaskAndRun(context.Background(), current, run); err != nil {
		return err
	}
	service.publish(context.Background(), current)
	return nil
}

func (service *Service) finishError(task domainsubagent.Task, run domainsubagent.Run, runContext context.Context, runErr error) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	current, err := service.Tasks.GetTask(context.Background(), task.ID)
	if err != nil {
		return err
	}
	if current.Terminal() {
		if current.Status == domainsubagent.TaskStatusCanceled || current.Status == domainsubagent.TaskStatusFailed {
			service.discardFailedWorkspace(task)
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
	run.CompletedAt = timePtr(now)
	if err := service.saveTaskAndRun(context.Background(), current, run); err != nil {
		return err
	}
	service.publish(context.Background(), current)
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
