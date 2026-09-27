package service

import (
	"context"
	"errors"
	"strings"
	"time"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

type AgentMessageAcknowledger struct {
	Repository subagentport.AgentMessageRepository
}

func (acknowledger AgentMessageAcknowledger) Acknowledge(messageID string) error {
	if acknowledger.Repository == nil {
		return nil
	}
	return acknowledger.Repository.MarkAgentMessageDelivered(context.Background(), messageID, time.Now().UTC())
}

// notifyParentCompletionLocked records a successful queue delivery in memory
// so a retry of the explicit resume endpoint does not enqueue the same result
// twice during the current process. The caller must hold service.mu.
func (service *Service) notifyParentCompletionLocked(task domainsubagent.Task) {
	if service == nil || service.ParentNotifier == nil || strings.TrimSpace(task.ParentSessionID) == "" {
		return
	}
	if service.parentNotified == nil {
		service.parentNotified = make(map[string]struct{})
	}
	if _, exists := service.parentNotified[task.ID]; exists {
		return
	}
	if service.AgentMessages != nil {
		message, err := service.prepareParentCompletionMessageLocked(task)
		if err != nil {
			service.reportError(err)
			return
		}
		if message.Status == domainsubagent.AgentMessageDelivered {
			service.parentNotified[task.ID] = struct{}{}
			return
		}
	}
	if err := service.ParentNotifier.Notify(context.Background(), domainsubagent.CloneTask(task)); err != nil {
		service.reportError(err)
		return
	}
	service.parentNotified[task.ID] = struct{}{}
}

func (service *Service) ensureParentCompletionQueued(task domainsubagent.Task) error {
	if service == nil || service.ParentNotifier == nil || strings.TrimSpace(task.ParentSessionID) == "" {
		return nil
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.parentNotified == nil {
		service.parentNotified = make(map[string]struct{})
	}
	if _, exists := service.parentNotified[task.ID]; exists {
		return nil
	}
	if service.AgentMessages != nil {
		message, err := service.prepareParentCompletionMessageLocked(task)
		if err != nil {
			return err
		}
		if message.Status == domainsubagent.AgentMessageDelivered {
			service.parentNotified[task.ID] = struct{}{}
			return nil
		}
	}
	if err := service.ParentNotifier.Notify(context.Background(), domainsubagent.CloneTask(task)); err != nil {
		return err
	}
	service.parentNotified[task.ID] = struct{}{}
	return nil
}

func (service *Service) prepareParentCompletionMessageLocked(task domainsubagent.Task) (domainsubagent.AgentMessage, error) {
	messageID := domainsubagent.AgentResultMessageID(task.ID)
	if existing, err := service.AgentMessages.GetAgentMessage(context.Background(), messageID); err == nil {
		return existing, nil
	} else if !errors.Is(err, subagentport.ErrNotFound) {
		return domainsubagent.AgentMessage{}, err
	}
	content := strings.TrimSpace(task.Result)
	kind := domainsubagent.AgentMessageKindTaskResult
	if task.Status == domainsubagent.TaskStatusFailed {
		content = strings.TrimSpace(task.ErrorMessage)
		kind = domainsubagent.AgentMessageKindTaskError
	}
	if content == "" {
		content = "subagent completed without a textual result"
	}
	message := domainsubagent.AgentMessage{
		ID: messageID, SourceTaskID: task.ID, AuthorAgentID: task.ChildSessionID,
		RecipientAgentID: task.ParentSessionID, ParentTurnID: task.ParentRunID,
		RootAgentID: task.ParentSessionID, Kind: kind, Content: content,
		Trigger: domainsubagent.AgentMessageTriggerQueue, Status: domainsubagent.AgentMessagePending,
		CreatedAt: service.now(),
	}
	if err := message.Validate(); err != nil {
		return domainsubagent.AgentMessage{}, err
	}
	if repository, ok := service.AgentMessages.(subagentport.ParentCompletionMessageRepository); ok {
		return repository.EnqueueParentCompletion(context.Background(), task.ID, message)
	}
	if err := service.AgentMessages.SaveAgentMessage(context.Background(), message); err != nil {
		return domainsubagent.AgentMessage{}, err
	}
	return message, nil
}

// RecoverPendingAgentMessages rehydrates parent input after a process restart.
// The durable message is the source of truth; the in-memory parent queue is
// only an execution optimization.
func (service *Service) RecoverPendingAgentMessages(ctx context.Context) error {
	if service == nil || service.AgentMessages == nil || service.ParentNotifier == nil || service.Tasks == nil {
		return nil
	}
	messages, err := service.AgentMessages.ListPendingParentMessages(ctx)
	if err != nil {
		return err
	}
	for _, message := range messages {
		if repository, ok := service.AgentMessages.(subagentport.ParentCompletionMessageRepository); ok {
			ownerID := service.MessageOwnerID
			if ownerID == "" {
				ownerID = "subagent-parent-recovery"
			}
			claimed, claimErr := repository.ClaimParentCompletion(ctx, message.ID, ownerID, service.now(), service.leaseTTL())
			if claimErr != nil {
				if errors.Is(claimErr, subagentport.ErrExecutionLeaseNotAcquired) || errors.Is(claimErr, subagentport.ErrTaskStateConflict) {
					continue
				}
				return claimErr
			}
			message = claimed
		}
		task, loadErr := service.Tasks.GetTask(ctx, message.SourceTaskID)
		if loadErr != nil {
			if repository, ok := service.AgentMessages.(subagentport.ParentCompletionMessageRepository); ok {
				_ = repository.ReleaseParentCompletion(ctx, message.ID, message.ClaimOwnerID, loadErr.Error())
			} else {
				_ = service.AgentMessages.MarkAgentMessageFailed(ctx, message.ID, loadErr.Error())
			}
			continue
		}
		service.mu.Lock()
		if err := service.ParentNotifier.Notify(ctx, domainsubagent.CloneTask(task)); err != nil {
			service.mu.Unlock()
			if repository, ok := service.AgentMessages.(subagentport.ParentCompletionMessageRepository); ok {
				_ = repository.ReleaseParentCompletion(ctx, message.ID, message.ClaimOwnerID, err.Error())
			} else {
				service.reportError(err)
			}
			continue
		}
		// Notify only enqueues the identified input. Release the recovery claim
		// back to Pending; the parent generation loop marks it Delivered only
		// after appending and consuming the input. A restart retries the enqueue,
		// which is idempotent by message ID.
		if repository, ok := service.AgentMessages.(subagentport.ParentCompletionMessageRepository); ok {
			if err := repository.ReleaseParentCompletion(ctx, message.ID, message.ClaimOwnerID, "queued for parent session"); err != nil {
				service.mu.Unlock()
				return err
			}
		} else if err := service.AgentMessages.MarkAgentMessageDelivered(ctx, message.ID, service.now()); err != nil {
			service.mu.Unlock()
			return err
		}
		if service.parentNotified == nil {
			service.parentNotified = make(map[string]struct{})
		}
		service.parentNotified[task.ID] = struct{}{}
		service.mu.Unlock()
	}
	return nil
}
