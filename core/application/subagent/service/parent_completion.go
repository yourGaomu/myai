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
	// OwnerID makes acknowledgement a lease-based completion instead of an
	// unconditional status update. It must identify this application instance.
	OwnerID  string
	LeaseTTL time.Duration
}

func (acknowledger AgentMessageAcknowledger) Acknowledge(messageID string) error {
	if acknowledger.Repository == nil {
		return nil
	}
	if parentRepository, ok := acknowledger.Repository.(subagentport.ParentCompletionMessageRepository); ok {
		// 1. 为当前进程确定稳定 owner，并创建短期 lease，防止多个 worker 同时确认。
		ownerID := strings.TrimSpace(acknowledger.OwnerID)
		if ownerID == "" {
			ownerID = "parent-message-ack"
		}
		ttl := acknowledger.LeaseTTL
		if ttl <= 0 {
			ttl = 2 * time.Minute
		}
		now := time.Now().UTC()
		if _, err := parentRepository.ClaimParentCompletion(context.Background(), messageID, ownerID, now, ttl); err != nil {
			// Completion is idempotent: another worker may have completed it
			// between the model response and this acknowledgement.
			if errors.Is(err, subagentport.ErrTaskStateConflict) {
				if message, getErr := acknowledger.Repository.GetAgentMessage(context.Background(), messageID); getErr == nil && message.Status == domainsubagent.AgentMessageDelivered {
					return nil
				}
			}
			return err
		}
		// 2. 只有 claim 成功后才将消息标记为 delivered；失败会保留为可重试状态。
		return parentRepository.CompleteParentCompletion(context.Background(), messageID, ownerID, now)
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
	// 1. 先复用已有 envelope，保证任务完成事件在重试时保持同一个消息 ID。
	messageID := domainsubagent.AgentResultMessageID(task.ID)
	if existing, err := service.AgentMessages.GetAgentMessage(context.Background(), messageID); err == nil {
		return existing, nil
	} else if !errors.Is(err, subagentport.ErrNotFound) {
		return domainsubagent.AgentMessage{}, err
	}
	// 2. 持久化内容必须与实际进入父会话的 continuation prompt 完全一致。
	content, contentErr := domainsubagent.CompletionMessageContent(task)
	if contentErr != nil {
		return domainsubagent.AgentMessage{}, contentErr
	}
	kind := domainsubagent.AgentMessageKindTaskResult
	if task.Status == domainsubagent.TaskStatusFailed {
		kind = domainsubagent.AgentMessageKindTaskError
	}
	message := domainsubagent.AgentMessage{
		ID: messageID, SourceTaskID: task.ID, AuthorAgentID: task.ChildSessionID,
		RecipientAgentID: task.ParentSessionID, ParentTurnID: task.ParentRunID,
		RootAgentID: task.ParentSessionID, Kind: kind, Content: content,
		Trigger: domainsubagent.AgentMessageTriggerTurn, Status: domainsubagent.AgentMessagePending,
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
		// 1. 进程启动后先领取 durable message，避免多个实例重复恢复同一事件。
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
		// 2. 恢复只负责重新放入父会话 mailbox，不在这里提前确认 delivered。
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
