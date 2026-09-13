package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	subagentcommand "myai/core/application/subagent/command"
	subagentresult "myai/core/application/subagent/result"
	domainsubagent "myai/core/domain/subagent"
	"myai/core/llm"
	"myai/core/remote/protocol"
	"myai/core/service"
)

func (agent *Agent) handleSubagentDefinitionList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if agent.subagentService == nil {
		return errors.New("subagent service is unavailable")
	}
	if _, err := protocol.DecodePayload[protocol.SubagentDefinitionListPayload](message); err != nil {
		return fmt.Errorf("decode subagent definition list: %w", err)
	}
	result, err := agent.subagentService.ListDefinitions(ctx)
	if err != nil {
		return err
	}
	return agent.writeRemoteMessage(conn, protocol.TypeSubagentDefinitionListResult, message.RequestID, message.SessionID, protocol.SubagentDefinitionListResultPayload{
		Definitions: subagentDefinitionPayloads(result.Items),
	})
}

func (agent *Agent) handleSubagentDefinitionCreate(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentDefinitionPayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent definition create: %w", err)
	}
	result, err := agent.subagentService.CreateDefinition(ctx, subagentcommand.CreateDefinition{Definition: subagentDefinitionFromPayload(payload, true)})
	if err != nil {
		return err
	}
	return agent.writeSubagentDefinitionMutation(ctx, conn, message, &result.Value, "Subagent definition created.", "")
}

func (agent *Agent) handleSubagentDefinitionUpdate(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentDefinitionPayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent definition update: %w", err)
	}
	result, err := agent.subagentService.UpdateDefinition(ctx, subagentcommand.UpdateDefinition{Definition: subagentDefinitionFromPayload(payload, false)})
	if err != nil {
		return err
	}
	return agent.writeSubagentDefinitionMutation(ctx, conn, message, &result.Value, "Subagent definition updated.", "")
}

func (agent *Agent) handleSubagentDefinitionDelete(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentDefinitionDeletePayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent definition delete: %w", err)
	}
	if err := agent.subagentService.DeleteDefinition(ctx, subagentcommand.DeleteDefinition{DefinitionID: payload.DefinitionID}); err != nil {
		return err
	}
	return agent.writeSubagentDefinitionMutation(ctx, conn, message, nil, "Subagent definition deleted.", strings.TrimSpace(payload.DefinitionID))
}

func (agent *Agent) writeSubagentDefinitionMutation(ctx context.Context, conn *websocket.Conn, message protocol.Message, definition *domainsubagent.Definition, text string, deletedID string) error {
	result, err := agent.subagentService.ListDefinitions(ctx)
	if err != nil {
		return err
	}
	payload := protocol.SubagentDefinitionMutationResultPayload{
		Definitions: subagentDefinitionPayloads(result.Items), DeletedID: deletedID, Message: text,
	}
	if definition != nil {
		mapped := subagentDefinitionPayload(*definition)
		payload.Definition = &mapped
	}
	return agent.writeRemoteMessage(conn, protocol.TypeSubagentDefinitionMutationResult, message.RequestID, message.SessionID, payload)
}

func (agent *Agent) handleSubagentTaskList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskListPayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent task list: %w", err)
	}
	sessionID := agent.subagentSessionID(message, payload.SessionID)
	result, err := agent.subagentService.List(ctx, subagentcommand.ListTasks{ParentSessionID: sessionID, Limit: payload.Limit})
	if err != nil {
		return err
	}
	return agent.writeRemoteMessage(conn, protocol.TypeSubagentTaskListResult, message.RequestID, sessionID, protocol.SubagentTaskListResultPayload{
		SessionID: sessionID, Tasks: subagentTaskPayloads(result.Items),
	})
}

func (agent *Agent) handleSubagentTaskCheck(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskPayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent task check: %w", err)
	}
	sessionID := agent.subagentSessionID(message, "")
	result, err := agent.subagentService.Check(ctx, subagentcommand.CheckTask{
		TaskID: payload.TaskID, ParentSessionID: sessionID, RequestID: message.RequestID,
	})
	if err != nil {
		return err
	}
	return agent.writeSubagentTaskResult(conn, message, result.Value, "Subagent task refreshed.")
}

func (agent *Agent) handleSubagentTaskMessage(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskMessagePayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent task message: %w", err)
	}
	result, err := agent.subagentService.SendMessage(ctx, subagentcommand.SendMessage{
		TaskID: payload.TaskID, ParentSessionID: agent.subagentSessionID(message, ""), Content: payload.Message,
	})
	if err != nil {
		return err
	}
	return agent.writeSubagentTaskResult(conn, message, result.Value, "Message queued for subagent.")
}

func (agent *Agent) handleSubagentTaskFollowup(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskMessagePayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent task follow-up: %w", err)
	}
	result, err := agent.subagentService.Followup(ctx, subagentcommand.FollowupTask{
		TaskID: payload.TaskID, ParentSessionID: agent.subagentSessionID(message, ""),
		RequestID: message.RequestID, Content: payload.Message,
	})
	if err != nil {
		return err
	}
	return agent.writeSubagentTaskResult(conn, message, result.Value, "Subagent follow-up scheduled.")
}

func (agent *Agent) handleSubagentTaskWait(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskWaitPayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent task wait: %w", err)
	}
	sessionID := agent.subagentSessionID(message, payload.SessionID)
	var timeout time.Duration
	if payload.TimeoutMS > 0 {
		timeout = time.Duration(payload.TimeoutMS) * time.Millisecond
	}
	result, err := agent.subagentService.Wait(ctx, subagentcommand.WaitTask{
		TaskID: payload.TaskID, ParentSessionID: sessionID, Timeout: timeout,
	})
	if err != nil {
		return err
	}
	return agent.writeRemoteMessage(conn, protocol.TypeSubagentTaskWaitResult, message.RequestID, sessionID, protocol.SubagentTaskWaitResultPayload{
		SessionID: sessionID, Task: subagentTaskPayload(result.Task), TimedOut: result.TimedOut, WokenByMailbox: result.WokenByMailbox, Sequence: result.Sequence,
	})
}

func (agent *Agent) handleSubagentTaskCancel(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskPayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent task cancel: %w", err)
	}
	result, err := agent.subagentService.Cancel(ctx, subagentcommand.CancelTask{
		TaskID: payload.TaskID, ParentSessionID: agent.subagentSessionID(message, ""), Reason: "canceled from mobile",
	})
	if err != nil {
		return err
	}
	return agent.writeSubagentTaskResult(conn, message, result.Value, "Subagent task canceled.")
}

func (agent *Agent) handleSubagentTaskApply(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskPayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent task apply: %w", err)
	}
	result, err := agent.subagentService.ApplyChanges(ctx, subagentcommand.ApplyTaskChanges{
		TaskID: payload.TaskID, ParentSessionID: agent.subagentSessionID(message, ""), RequestID: message.RequestID,
	})
	if err != nil {
		return err
	}
	return agent.writeSubagentTaskResult(conn, message, result.Value, "Subagent changes applied.")
}

func (agent *Agent) handleSubagentTaskDiscard(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskPayload](message)
	if err != nil {
		return fmt.Errorf("decode subagent task discard: %w", err)
	}
	result, err := agent.subagentService.DiscardChanges(ctx, subagentcommand.DiscardTaskChanges{
		TaskID: payload.TaskID, ParentSessionID: agent.subagentSessionID(message, ""),
	})
	if err != nil {
		return err
	}
	return agent.writeSubagentTaskResult(conn, message, result.Value, "Subagent changes discarded.")
}

func (agent *Agent) processSubagentTaskResume(ctx context.Context, conn *websocket.Conn, message protocol.Message) {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskPayload](message)
	if err != nil {
		agent.writeSubagentResumeError(conn, message, fmt.Errorf("decode subagent task resume: %w", err))
		return
	}
	sessionID := agent.subagentSessionID(message, "")
	if sessionID == "" {
		agent.writeSubagentResumeError(conn, message, errors.New("session id is empty"))
		return
	}

	runtime := agent.runtimes.get(sessionID)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runCtx, cancel, ok := runtime.start(ctx)
	if !ok {
		agent.writeSubagentResumeError(conn, message, errors.New("session is already running"))
		return
	}
	defer runtime.finish(cancel)

	result, err := agent.handleSubagentTaskResume(runCtx, conn, message, sessionID, payload.TaskID)
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(runCtx.Err(), context.Canceled) {
		if writeErr := agent.writePausedAssistantDone(conn, message.RequestID, sessionID); writeErr != nil {
			log.Printf("send paused subagent continuation failed: %v", writeErr)
			return
		}
		if result.Task.ID == "" {
			agent.writeSubagentResumeError(conn, message, err)
			return
		}
		if writeErr := agent.writeSubagentTaskResumeResult(conn, message, result.Task, "Parent continuation paused. The subagent result is available to retry."); writeErr != nil {
			log.Printf("send paused subagent resume result failed: %v", writeErr)
		}
		return
	}
	agent.writeSubagentResumeError(conn, message, err)
}

func (agent *Agent) handleSubagentTaskResume(ctx context.Context, conn *websocket.Conn, message protocol.Message, sessionID string, taskID string) (subagentresult.Resume, error) {
	if agent.subagentService == nil {
		return subagentresult.Resume{}, errors.New("subagent service is unavailable")
	}
	var resumed subagentresult.Resume
	response, err := agent.streamChatResponse(ctx, conn, message, sessionID, func(stream llm.ChatStreamHandler) (service.ChatResponse, error) {
		var resumeErr error
		resumed, resumeErr = agent.subagentService.Resume(ctx, subagentcommand.ResumeTask{
			TaskID: taskID, ParentSessionID: sessionID, Stream: stream,
		})
		return service.ChatResponse{
			SessionID: resumed.Task.ParentSessionID,
			Result:    llm.ChatResult{Content: resumed.Content, Reasoning: resumed.Reasoning, Usage: resumed.Usage},
		}, resumeErr
	})
	if err != nil {
		return resumed, err
	}
	if err := agent.writeRemoteMessage(conn, protocol.TypeAssistantDone, message.RequestID, response.SessionID, protocol.AssistantDonePayload{
		Content: response.Result.Content, Reasoning: response.Result.Reasoning, Usage: tokenUsagePayload(response.Result.Usage),
	}); err != nil {
		return resumed, err
	}
	if err := agent.writeSubagentTaskResumeResult(conn, message, resumed.Task, "Parent session continued from the subagent result."); err != nil {
		return resumed, err
	}
	return resumed, nil
}

func (agent *Agent) writeSubagentResumeError(conn *websocket.Conn, message protocol.Message, err error) {
	if err == nil {
		return
	}
	if writeErr := agent.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.ErrorPayload{Message: err.Error()}); writeErr != nil {
		log.Printf("send subagent resume error failed: %v", writeErr)
	}
}

func (agent *Agent) writeSubagentTaskResumeResult(conn *websocket.Conn, message protocol.Message, task domainsubagent.Task, text string) error {
	return agent.writeRemoteMessage(conn, protocol.TypeSubagentTaskResumeResult, message.RequestID, task.ParentSessionID, protocol.SubagentTaskResultPayload{
		Task: subagentTaskPayload(task), Message: text,
	})
}

func (agent *Agent) writeSubagentTaskResult(conn *websocket.Conn, message protocol.Message, task domainsubagent.Task, text string) error {
	return agent.writeRemoteMessage(conn, protocol.TypeSubagentTaskResult, message.RequestID, task.ParentSessionID, protocol.SubagentTaskResultPayload{
		Task: subagentTaskPayload(task), Message: text,
	})
}

func (agent *Agent) subagentSessionID(message protocol.Message, payloadSessionID string) string {
	if value := strings.TrimSpace(payloadSessionID); value != "" {
		return value
	}
	if value := strings.TrimSpace(message.SessionID); value != "" {
		return value
	}
	return agent.chatService.CurrentSessionID()
}
