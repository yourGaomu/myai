package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
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
		// Each attempt owns a distinct claim, including simultaneous ACKs on
		// the same process and configurations without an instance owner ID.
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return err
		}
		ownerID += ":" + fmt.Sprintf("%x", nonce)
		ttl := acknowledger.LeaseTTL
		if ttl <= 0 {
			ttl = 2 * time.Minute
		}
		now := time.Now().UTC()
		if _, err := parentRepository.ClaimParentCompletion(context.Background(), messageID, ownerID, now, ttl); err != nil {
			// Completion is idempotent: another worker may have completed it
			// between the model response and this acknowledgement.
			if errors.Is(err, subagentport.ErrTaskStateConflict) || errors.Is(err, subagentport.ErrExecutionLeaseNotAcquired) {
				if message, getErr := acknowledger.Repository.GetAgentMessage(context.Background(), messageID); getErr == nil && message.Status == domainsubagent.AgentMessageDelivered {
					return nil
				}
			}
			return err
		}
		// A failed commit releases this attempt's claim so a retry does not
		// wait for the lease TTL. Release cannot undo an already committed ACK.
		if err := parentRepository.CompleteParentCompletion(context.Background(), messageID, ownerID, time.Now().UTC()); err != nil {
			_ = parentRepository.ReleaseParentCompletion(context.Background(), messageID, ownerID, err.Error())
			return err
		}
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
	messageID := domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID)
	if _, exists := service.parentNotified[messageID]; exists {
		return
	}
	message, err := service.prepareParentCompletionMessageLocked(task)
	if err != nil {
		service.reportError(err)
		return
	}
	if message.Status == domainsubagent.AgentMessageDelivered {
		service.parentNotified[messageID] = struct{}{}
		return
	}
	if err := service.ParentNotifier.Notify(context.Background(), message); err != nil {
		service.reportError(err)
		return
	}
	service.parentNotified[messageID] = struct{}{}
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
	messageID := domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID)
	if _, exists := service.parentNotified[messageID]; exists {
		return nil
	}
	message, err := service.prepareParentCompletionMessageLocked(task)
	if err != nil {
		return err
	}
	if message.Status == domainsubagent.AgentMessageDelivered {
		service.parentNotified[messageID] = struct{}{}
		return nil
	}
	if err := service.ParentNotifier.Notify(context.Background(), message); err != nil {
		return err
	}
	service.parentNotified[messageID] = struct{}{}
	return nil
}

func (service *Service) prepareParentCompletionMessageLocked(task domainsubagent.Task) (domainsubagent.AgentMessage, error) {
	// 1. 先复用已有 envelope，保证任务完成事件在重试时保持同一个消息 ID。
	messageID := domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID)
	if service.AgentMessages != nil {
		if existing, err := service.AgentMessages.GetAgentMessage(context.Background(), messageID); err == nil {
			return service.boundParentCompletion(context.Background(), existing)
		} else if !errors.Is(err, subagentport.ErrNotFound) {
			return domainsubagent.AgentMessage{}, err
		}
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
	if service.AgentMessages == nil {
		return message, nil
	}
	if repository, ok := service.AgentMessages.(subagentport.ParentCompletionMessageRepository); ok {
		return repository.EnqueueParentCompletion(context.Background(), task.ID, message)
	}
	if err := service.AgentMessages.SaveAgentMessage(context.Background(), message); err != nil {
		return domainsubagent.AgentMessage{}, err
	}
	return message, nil
}

func (service *Service) boundParentCompletion(ctx context.Context, message domainsubagent.AgentMessage) (domainsubagent.AgentMessage, error) {
	bounded, err := domainsubagent.BoundParentCompletionMessage(message)
	if err != nil || bounded.Content == message.Content {
		return bounded, err
	}
	if repository, ok := service.AgentMessages.(subagentport.ParentCompletionContentRepository); ok {
		return repository.BoundParentCompletion(ctx, message.ID)
	}
	return bounded, nil
}

// RecoverPendingAgentMessages rehydrates parent input after a process restart.
// The durable message is the source of truth; the in-memory parent queue is
// only an execution optimization.
func (service *Service) RecoverPendingAgentMessages(ctx context.Context) error {
	if service == nil || service.AgentMessages == nil || service.ParentNotifier == nil {
		return nil
	}
	messages, err := service.AgentMessages.ListPendingParentMessages(ctx)
	if err != nil {
		return err
	}
	for _, message := range messages {
		message, err = service.boundParentCompletion(ctx, message)
		if err != nil {
			return err
		}
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
		service.mu.Lock()
		// 2. 恢复只负责重新放入父会话 mailbox，不在这里提前确认 delivered。
		// Replay the persisted envelope, never rebuild it from a newer task run.
		if err := service.ParentNotifier.Notify(ctx, domainsubagent.CloneAgentMessage(message)); err != nil {
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
		service.parentNotified[message.ID] = struct{}{}
		service.mu.Unlock()
	}
	return nil
}
