package mapper

import (
	"time"

	"myai/core/adapter/persistence/mongo/subagent/po"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
)

func DefinitionDocumentFromDomain(definition domainsubagent.Definition) po.DefinitionDocument {
	return po.DefinitionDocument{
		ID: definition.ID, Name: definition.Name, Description: definition.Description,
		SystemPrompt: definition.SystemPrompt, ModelID: definition.ModelID,
		AllowedTools:   append([]string(nil), definition.AllowedTools...),
		CapabilityMode: string(definition.CapabilityMode), IsolationMode: string(definition.IsolationMode),
		MaxTurns: definition.MaxTurns, TimeoutSeconds: definition.TimeoutSeconds,
		Enabled: definition.Enabled, Version: definition.Version, Source: string(definition.Source),
		Deleted: definition.Deleted, DeletedAt: cloneTime(definition.DeletedAt),
		CreatedAt: definition.CreatedAt, UpdatedAt: definition.UpdatedAt,
	}
}

func DefinitionDomainFromDocument(document po.DefinitionDocument) domainsubagent.Definition {
	return domainsubagent.Definition{
		ID: document.ID, Name: document.Name, Description: document.Description,
		SystemPrompt: document.SystemPrompt, ModelID: document.ModelID,
		AllowedTools:   append([]string(nil), document.AllowedTools...),
		CapabilityMode: domainsubagent.CapabilityMode(document.CapabilityMode),
		IsolationMode:  domainworkspace.IsolationMode(document.IsolationMode),
		MaxTurns:       document.MaxTurns, TimeoutSeconds: document.TimeoutSeconds,
		Enabled: document.Enabled, Version: document.Version, Source: domainsubagent.DefinitionSource(document.Source),
		Deleted: document.Deleted, DeletedAt: cloneTime(document.DeletedAt),
		CreatedAt: document.CreatedAt, UpdatedAt: document.UpdatedAt,
	}
}

func TaskDocumentFromDomain(task domainsubagent.Task) po.TaskDocument {
	return po.TaskDocument{
		ID: task.ID, ParentSessionID: task.ParentSessionID, ChildSessionID: task.ChildSessionID,
		CreatedRequestID: task.CreatedRequestID, DefinitionID: task.DefinitionID,
		DefinitionVersion: task.DefinitionVersion, Definition: snapshotDocument(task.Definition),
		Instruction: task.Instruction, Title: task.Title, Status: string(task.Status), CurrentRunID: task.CurrentRunID,
		Workspace: workspaceDocument(task.Workspace), Result: task.Result, Reasoning: task.Reasoning,
		ChangeSet:    changeSetDocument(task.ChangeSet),
		ErrorMessage: task.ErrorMessage, Unread: task.Unread, CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt,
		StartedAt: cloneTime(task.StartedAt), CompletedAt: cloneTime(task.CompletedAt),
	}
}

func TaskDomainFromDocument(document po.TaskDocument) domainsubagent.Task {
	unread := document.Unread
	if domainsubagent.TaskStatus(document.Status) == domainsubagent.TaskStatusCanceled {
		unread = false
	}
	return domainsubagent.Task{
		ID: document.ID, ParentSessionID: document.ParentSessionID, ChildSessionID: document.ChildSessionID,
		CreatedRequestID: document.CreatedRequestID, DefinitionID: document.DefinitionID,
		DefinitionVersion: document.DefinitionVersion, Definition: snapshotDomain(document.Definition),
		Instruction: document.Instruction, Title: document.Title, Status: domainsubagent.TaskStatus(document.Status),
		CurrentRunID: document.CurrentRunID, Workspace: workspaceDomain(document.Workspace),
		ChangeSet: changeSetDomain(document.ChangeSet),
		Result:    document.Result, Reasoning: document.Reasoning, ErrorMessage: document.ErrorMessage,
		Unread: unread, CreatedAt: document.CreatedAt, UpdatedAt: document.UpdatedAt,
		StartedAt: cloneTime(document.StartedAt), CompletedAt: cloneTime(document.CompletedAt),
	}
}

func RunDocumentFromDomain(run domainsubagent.Run) po.RunDocument {
	return po.RunDocument{
		ID: run.ID, TaskID: run.TaskID, Sequence: run.Sequence, Instruction: run.Instruction,
		Status: string(run.Status), Result: run.Result, ErrorMessage: run.ErrorMessage,
		CreatedAt: run.CreatedAt, StartedAt: cloneTime(run.StartedAt), CompletedAt: cloneTime(run.CompletedAt),
	}
}

func RunDomainFromDocument(document po.RunDocument) domainsubagent.Run {
	return domainsubagent.Run{
		ID: document.ID, TaskID: document.TaskID, Sequence: document.Sequence, Instruction: document.Instruction,
		Status: domainsubagent.RunStatus(document.Status), Result: document.Result, ErrorMessage: document.ErrorMessage,
		CreatedAt: document.CreatedAt, StartedAt: cloneTime(document.StartedAt), CompletedAt: cloneTime(document.CompletedAt),
	}
}

func snapshotDocument(snapshot domainsubagent.DefinitionSnapshot) po.DefinitionSnapshotDocument {
	return po.DefinitionSnapshotDocument{
		ID: snapshot.ID, Name: snapshot.Name, SystemPrompt: snapshot.SystemPrompt, ModelID: snapshot.ModelID,
		AllowedTools: append([]string(nil), snapshot.AllowedTools...), CapabilityMode: string(snapshot.CapabilityMode),
		IsolationMode: string(snapshot.IsolationMode), MaxTurns: snapshot.MaxTurns,
		TimeoutSeconds: snapshot.TimeoutSeconds, Version: snapshot.Version,
	}
}

func snapshotDomain(document po.DefinitionSnapshotDocument) domainsubagent.DefinitionSnapshot {
	return domainsubagent.DefinitionSnapshot{
		ID: document.ID, Name: document.Name, SystemPrompt: document.SystemPrompt, ModelID: document.ModelID,
		AllowedTools:   append([]string(nil), document.AllowedTools...),
		CapabilityMode: domainsubagent.CapabilityMode(document.CapabilityMode),
		IsolationMode:  domainworkspace.IsolationMode(document.IsolationMode),
		MaxTurns:       document.MaxTurns, TimeoutSeconds: document.TimeoutSeconds, Version: document.Version,
	}
}

func workspaceDocument(reference domainworkspace.Reference) po.WorkspaceReferenceDocument {
	return po.WorkspaceReferenceDocument{ID: reference.ID, Mode: string(reference.Mode), Root: reference.Root, SandboxID: reference.SandboxID}
}

func workspaceDomain(document po.WorkspaceReferenceDocument) domainworkspace.Reference {
	return domainworkspace.Reference{ID: document.ID, Mode: domainworkspace.IsolationMode(document.Mode), Root: document.Root, SandboxID: document.SandboxID}
}

func changeSetDocument(changeSet domainworkspace.ChangeSet) po.ChangeSetDocument {
	files := make([]po.FileChangeDocument, 0, len(changeSet.Files))
	for _, file := range changeSet.Files {
		files = append(files, po.FileChangeDocument{
			Path: file.Path, ChangeType: string(file.ChangeType), BeforeHash: file.BeforeHash,
			AfterHash: file.AfterHash, BeforeSize: file.BeforeSize, AfterSize: file.AfterSize,
		})
	}
	return po.ChangeSetDocument{
		WorkspaceID: changeSet.WorkspaceID, Status: string(changeSet.Status), Files: files,
		CheckpointID: changeSet.CheckpointID, Message: changeSet.Message, CreatedAt: changeSet.CreatedAt,
		AppliedAt: cloneTime(changeSet.AppliedAt), DiscardedAt: cloneTime(changeSet.DiscardedAt),
	}
}

func changeSetDomain(document po.ChangeSetDocument) domainworkspace.ChangeSet {
	files := make([]domainworkspace.FileChange, 0, len(document.Files))
	for _, file := range document.Files {
		files = append(files, domainworkspace.FileChange{
			Path: file.Path, ChangeType: domainworkspace.FileChangeType(file.ChangeType),
			BeforeHash: file.BeforeHash, AfterHash: file.AfterHash,
			BeforeSize: file.BeforeSize, AfterSize: file.AfterSize,
		})
	}
	return domainworkspace.ChangeSet{
		WorkspaceID: document.WorkspaceID, Status: domainworkspace.ChangeSetStatus(document.Status), Files: files,
		CheckpointID: document.CheckpointID, Message: document.Message, CreatedAt: document.CreatedAt,
		AppliedAt: cloneTime(document.AppliedAt), DiscardedAt: cloneTime(document.DiscardedAt),
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
