package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/gorilla/websocket"
	subagentapi "myai/core/application/subagent/api"
	subagentquery "myai/core/application/subagent/query"
	"myai/core/remote/protocol"
)

func (a *Agent) handleSubagentTaskDetail(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskDetailPayload](message)
	if err != nil {
		return err
	}
	inspector, ok := a.subagentService.(subagentapi.InspectionService)
	if !ok {
		return errors.New("subagent inspection is unavailable")
	}
	// 1. 必须明确父会话；打开详情不使用 LoadSession，不切换主聊天的当前会话。
	parentID := strings.TrimSpace(message.SessionID)
	inspection, err := inspector.InspectTask(ctx, subagentquery.InspectTask{ParentSessionID: parentID, TaskID: payload.TaskID})
	if err != nil {
		return err
	}
	result := protocol.SubagentTaskDetailResultPayload{
		Task: subagentTaskPayload(inspection.Task), ReadOnly: true, SessionReady: inspection.SessionReady,
		EventSequence: inspection.EventSequence, Messages: []protocol.SessionHistoryMessage{},
		Runs: []protocol.SubagentRunSummary{}, AgentRuns: []protocol.AgentRunSnapshot{}, Assets: []protocol.AssetSummary{},
	}
	for _, run := range inspection.Runs {
		result.Runs = append(result.Runs, protocol.SubagentRunSummary{
			ID: run.ID, Sequence: run.Sequence, Instruction: run.Instruction, Status: string(run.Status),
			Result: run.Result, ErrorMessage: run.ErrorMessage, CreatedAt: run.CreatedAt, StartedAt: run.StartedAt, CompletedAt: run.CompletedAt,
		})
	}
	if inspection.SessionReady {
		if a.chatService == nil {
			return errors.New("chat queries are unavailable")
		}
		// 2. 复用只读查询服务，历史消息按稳定消息 ID 分页。
		limit := payload.MessageLimit
		if limit <= 0 {
			limit = 100
		}
		if limit > 200 {
			limit = 200
		}
		childID := inspection.Task.ChildSessionID
		messages, reset, err := a.chatService.ListSessionMessagesAfter(ctx, childID, payload.AfterMessageID, limit+1)
		if err != nil {
			return err
		}
		result.FullSyncRequired = reset
		if len(messages) > limit {
			result.HasMoreMessages = true
			messages = messages[:limit]
		}
		result.Messages = sessionHistoryMessages(messages)
		if len(result.Messages) > 0 {
			result.NextMessageID = result.Messages[len(result.Messages)-1].ID
		}
		// 3. 运行快照和资源属于子会话；事件回放另走游标接口，不混入父消息历史。
		runs, err := a.chatService.ListAgentRuns(ctx, childID, 20)
		if err != nil {
			return err
		}
		result.AgentRuns = agentRunSnapshotsPayload(runs)
		assets, err := a.chatService.ListAssets(ctx, childID, 100)
		if err != nil {
			return err
		}
		result.Assets = assetSummaries(assets)
	}
	return a.writeRemoteMessage(conn, protocol.TypeSubagentTaskDetailResult, message.RequestID, parentID, result)
}

func (a *Agent) handleSubagentTaskEvents(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.SubagentTaskEventsPayload](message)
	if err != nil {
		return err
	}
	inspector, ok := a.subagentService.(subagentapi.InspectionService)
	if !ok {
		return errors.New("subagent inspection is unavailable")
	}
	// 1. 与实时推送使用同一映射，避免实时页面与历史页面字段不一致。
	page, err := inspector.ListTaskEvents(ctx, subagentquery.TaskEvents{
		ParentSessionID: message.SessionID, TaskID: payload.TaskID, RunID: payload.RunID,
		AfterSequence: payload.AfterSequence, ThroughSequence: payload.ThroughSequence, Limit: payload.Limit,
	})
	if err != nil {
		return err
	}
	events := make([]protocol.SubagentTaskResultPayload, 0, len(page.Events))
	for _, event := range page.Events {
		events = append(events, subagentEventPayload(event))
	}
	return a.writeRemoteMessage(conn, protocol.TypeSubagentTaskEventsResult, message.RequestID, message.SessionID, protocol.SubagentTaskEventsResultPayload{
		SessionID: message.SessionID, TaskID: payload.TaskID, RunID: payload.RunID, Events: events,
		NextSequence: page.NextSequence, ThroughSequence: page.ThroughSequence, HasMore: page.HasMore,
	})
}
