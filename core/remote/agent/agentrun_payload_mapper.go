package agent

import (
	agentrunresult "myai/core/application/agentrun/result"
	domainagentrun "myai/core/domain/agentrun"
	"myai/core/remote/protocol"
)

func agentRunPayload(run domainagentrun.Run) protocol.AgentRun {
	return protocol.AgentRun{
		ID: run.ID, RequestID: run.RequestID, SessionID: run.SessionID, ParentRunID: run.ParentRunID, PlanID: run.PlanID, StepID: run.StepID, TaskID: run.TaskID, Kind: string(run.Kind),
		Title: run.Title, Reason: run.Reason, Status: string(run.Status), CurrentStep: run.CurrentStep,
		TotalSteps: run.TotalSteps, LastSequence: run.LastSequence, ErrorMessage: run.ErrorMessage,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
}

func agentRunEventPayload(event domainagentrun.Event) protocol.AgentRunEvent {
	return protocol.AgentRunEvent{
		ID: event.ID, RunID: event.RunID, SessionID: event.SessionID, Sequence: event.Sequence,
		Type: string(event.Type), Title: event.Title, Content: event.Content, ToolName: event.ToolName,
		Arguments: event.Arguments, Status: event.Status, ErrorCode: event.ErrorCode,
		ErrorMessage: event.ErrorMessage, Truncated: event.Truncated, Delta: event.Delta,
		CurrentStep: event.CurrentStep, TotalSteps: event.TotalSteps, CreatedAt: event.CreatedAt,
	}
}

func agentRunSnapshotsPayload(snapshots []agentrunresult.Snapshot) []protocol.AgentRunSnapshot {
	result := make([]protocol.AgentRunSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		events := make([]protocol.AgentRunEvent, 0, len(snapshot.Events))
		for _, event := range snapshot.Events {
			events = append(events, agentRunEventPayload(event))
		}
		result = append(result, protocol.AgentRunSnapshot{Run: agentRunPayload(snapshot.Run), Events: events})
	}
	return result
}
