package mapper

import (
	"myai/core/adapter/persistence/mongo/agentrun/po"
	domainagentrun "myai/core/domain/agentrun"
)

func RunDocumentFromDomain(run domainagentrun.Run) po.RunDocument {
	return po.RunDocument{
		ID: run.ID, RequestID: run.RequestID, SessionID: run.SessionID, ParentRunID: run.ParentRunID, PlanID: run.PlanID, StepID: run.StepID, TaskID: run.TaskID, Kind: string(run.Kind),
		Title: run.Title, Reason: run.Reason, Status: string(run.Status), CurrentStep: run.CurrentStep,
		TotalSteps: run.TotalSteps, LastSequence: run.LastSequence, ErrorMessage: run.ErrorMessage,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
}

func RunDomainFromDocument(document po.RunDocument) domainagentrun.Run {
	return domainagentrun.Run{
		ID: document.ID, RequestID: document.RequestID, SessionID: document.SessionID, ParentRunID: document.ParentRunID, PlanID: document.PlanID, StepID: document.StepID, TaskID: document.TaskID,
		Kind: domainagentrun.Kind(document.Kind), Title: document.Title, Reason: document.Reason,
		Status: domainagentrun.Status(document.Status), CurrentStep: document.CurrentStep,
		TotalSteps: document.TotalSteps, LastSequence: document.LastSequence,
		ErrorMessage: document.ErrorMessage, StartedAt: document.StartedAt, FinishedAt: document.FinishedAt,
	}
}

func EventDocumentFromDomain(event domainagentrun.Event) po.EventDocument {
	return po.EventDocument{
		ID: event.ID, RunID: event.RunID, SessionID: event.SessionID, Sequence: event.Sequence,
		Type: string(event.Type), Title: event.Title, Content: event.Content, ToolName: event.ToolName,
		Arguments: event.Arguments, Status: event.Status, ErrorCode: event.ErrorCode,
		ErrorMessage: event.ErrorMessage, Truncated: event.Truncated, CurrentStep: event.CurrentStep,
		TotalSteps: event.TotalSteps, CreatedAt: event.CreatedAt,
	}
}

func EventDomainFromDocument(document po.EventDocument) domainagentrun.Event {
	return domainagentrun.Event{
		ID: document.ID, RunID: document.RunID, SessionID: document.SessionID,
		Sequence: document.Sequence, Type: domainagentrun.EventType(document.Type), Title: document.Title,
		Content: document.Content, ToolName: document.ToolName, Arguments: document.Arguments,
		Status: document.Status, ErrorCode: document.ErrorCode, ErrorMessage: document.ErrorMessage,
		Truncated: document.Truncated, CurrentStep: document.CurrentStep, TotalSteps: document.TotalSteps,
		CreatedAt: document.CreatedAt,
	}
}
