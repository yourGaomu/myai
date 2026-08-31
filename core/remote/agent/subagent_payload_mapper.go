package agent

import (
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	"myai/core/remote/protocol"
)

func subagentDefinitionFromPayload(payload protocol.SubagentDefinitionPayload, create bool) domainsubagent.Definition {
	enabled := false
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	} else if create {
		enabled = true
	}
	return domainsubagent.Definition{
		ID: payload.ID, Name: payload.Name, Description: payload.Description,
		SystemPrompt: payload.SystemPrompt, ModelID: payload.ModelID,
		AllowedTools:   append([]string(nil), payload.AllowedTools...),
		CapabilityMode: domainsubagent.CapabilityMode(payload.CapabilityMode),
		IsolationMode:  domainworkspace.IsolationMode(payload.IsolationMode),
		MaxTurns:       payload.MaxTurns, TimeoutSeconds: payload.TimeoutSeconds, Enabled: enabled,
	}
}

func subagentDefinitionPayload(definition domainsubagent.Definition) protocol.SubagentDefinitionPayload {
	enabled := definition.Enabled
	return protocol.SubagentDefinitionPayload{
		ID: definition.ID, Name: definition.Name, Description: definition.Description,
		SystemPrompt: definition.SystemPrompt, ModelID: definition.ModelID,
		AllowedTools:   append([]string(nil), definition.AllowedTools...),
		CapabilityMode: string(definition.CapabilityMode), IsolationMode: string(definition.IsolationMode),
		MaxTurns: definition.MaxTurns, TimeoutSeconds: definition.TimeoutSeconds, Enabled: &enabled,
		Version: definition.Version, Source: string(definition.Source), CreatedAt: definition.CreatedAt, UpdatedAt: definition.UpdatedAt,
	}
}

func subagentDefinitionPayloads(definitions []domainsubagent.Definition) []protocol.SubagentDefinitionPayload {
	items := make([]protocol.SubagentDefinitionPayload, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, subagentDefinitionPayload(definition))
	}
	return items
}

func subagentTaskPayload(task domainsubagent.Task) protocol.SubagentTaskSummary {
	files := make([]protocol.SubagentFileChange, 0, len(task.ChangeSet.Files))
	for _, file := range task.ChangeSet.Files {
		files = append(files, protocol.SubagentFileChange{
			Path: file.Path, ChangeType: string(file.ChangeType), BeforeHash: file.BeforeHash,
			AfterHash: file.AfterHash, BeforeSize: file.BeforeSize, AfterSize: file.AfterSize,
		})
	}
	return protocol.SubagentTaskSummary{
		ID: task.ID, ParentSessionID: task.ParentSessionID, ParentTaskID: task.ParentTaskID, ParentRunID: task.ParentRunID, PlanID: task.PlanID, StepID: task.StepID,
		AgentPath: task.AgentPath, AgentNickname: task.AgentNickname, DefinitionID: task.DefinitionID,
		DefinitionVersion: task.DefinitionVersion, Title: task.Title, Instruction: task.Instruction,
		Status: string(task.Status), Unread: task.Unread, Result: task.Result, ErrorMessage: task.ErrorMessage,
		ChangeSet: protocol.SubagentChangeSet{
			WorkspaceID: task.ChangeSet.WorkspaceID, Status: string(task.ChangeSet.Status), Files: files,
			CheckpointID: task.ChangeSet.CheckpointID, Message: task.ChangeSet.Message,
			CreatedAt: task.ChangeSet.CreatedAt, AppliedAt: task.ChangeSet.AppliedAt, DiscardedAt: task.ChangeSet.DiscardedAt,
		},
		CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt, StartedAt: task.StartedAt, CompletedAt: task.CompletedAt,
	}
}

func subagentTaskPayloads(tasks []domainsubagent.Task) []protocol.SubagentTaskSummary {
	items := make([]protocol.SubagentTaskSummary, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, subagentTaskPayload(task))
	}
	return items
}
