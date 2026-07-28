package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gorilla/websocket"

	subagentcommand "myai/core/application/subagent/command"
	domainsubagent "myai/core/domain/subagent"
	"myai/core/remote/protocol"
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
