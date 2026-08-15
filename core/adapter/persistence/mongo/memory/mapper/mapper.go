package mapper

import (
	"myai/core/adapter/persistence/mongo/memory/po"
	domainmemory "myai/core/domain/memory"
)

func MemoryDocumentFromDomain(memory domainmemory.Memory) po.MemoryDocument {
	revisions := make([]po.RevisionDocument, 0, len(memory.Revisions))
	for _, revision := range memory.Revisions {
		revisions = append(revisions, revisionDocumentFromDomain(revision))
	}
	return po.MemoryDocument{
		ID: memory.ID, Title: memory.Title, Kind: string(memory.Kind), Scope: scopeDocumentFromDomain(memory.Scope),
		Tags: append([]string(nil), memory.Tags...), Status: string(memory.Status), CurrentVersion: memory.CurrentVersion,
		Revisions: revisions, HumanLocked: memory.HumanLocked, SupersedesID: memory.SupersedesID,
		UseCount: memory.UseCount, LastUsedAt: memory.LastUsedAt, DeletedAt: memory.DeletedAt,
		DeletionReason: memory.DeletionReason, CreatedAt: memory.CreatedAt, UpdatedAt: memory.UpdatedAt,
	}
}

func MemoryDomainFromDocument(document po.MemoryDocument) domainmemory.Memory {
	revisions := make([]domainmemory.Revision, 0, len(document.Revisions))
	for _, revision := range document.Revisions {
		revisions = append(revisions, revisionDomainFromDocument(revision))
	}
	return domainmemory.Memory{
		ID: document.ID, Title: document.Title, Kind: domainmemory.Kind(document.Kind), Scope: scopeDomainFromDocument(document.Scope),
		Tags: append([]string(nil), document.Tags...), Status: domainmemory.Status(document.Status), CurrentVersion: document.CurrentVersion,
		Revisions: revisions, HumanLocked: document.HumanLocked, SupersedesID: document.SupersedesID,
		UseCount: document.UseCount, LastUsedAt: document.LastUsedAt, DeletedAt: document.DeletedAt,
		DeletionReason: document.DeletionReason, CreatedAt: document.CreatedAt, UpdatedAt: document.UpdatedAt,
	}
}

func CandidateDocumentFromDomain(candidate domainmemory.Candidate) po.CandidateDocument {
	return po.CandidateDocument{
		ID: candidate.ID, OriginKey: candidate.OriginKey, Title: candidate.Title, Kind: string(candidate.Kind),
		Scope: scopeDocumentFromDomain(candidate.Scope), Tags: append([]string(nil), candidate.Tags...),
		Content: contentDocumentFromDomain(candidate.Content), Confidence: candidate.Confidence,
		Sources: sourceDocumentsFromDomain(candidate.Sources), Status: string(candidate.Status),
		TargetMemoryID: candidate.TargetMemoryID, ReviewNote: candidate.ReviewNote,
		CreatedAt: candidate.CreatedAt, UpdatedAt: candidate.UpdatedAt,
	}
}

func CandidateDomainFromDocument(document po.CandidateDocument) domainmemory.Candidate {
	return domainmemory.Candidate{
		ID: document.ID, OriginKey: document.OriginKey, Title: document.Title, Kind: domainmemory.Kind(document.Kind),
		Scope: scopeDomainFromDocument(document.Scope), Tags: append([]string(nil), document.Tags...),
		Content: contentDomainFromDocument(document.Content), Confidence: document.Confidence,
		Sources: sourceDomainsFromDocument(document.Sources), Status: domainmemory.CandidateStatus(document.Status),
		TargetMemoryID: document.TargetMemoryID, ReviewNote: document.ReviewNote,
		CreatedAt: document.CreatedAt, UpdatedAt: document.UpdatedAt,
	}
}

func ExtractionJobDocumentFromDomain(job domainmemory.ExtractionJob) po.ExtractionJobDocument {
	return po.ExtractionJobDocument{
		ID: job.ID, AgentRunID: job.AgentRunID, ExtractorVersion: job.ExtractorVersion,
		Status: string(job.Status), Attempts: job.Attempts, LastError: job.LastError,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt, CompletedAt: job.CompletedAt,
	}
}

func ExtractionJobDomainFromDocument(document po.ExtractionJobDocument) domainmemory.ExtractionJob {
	return domainmemory.ExtractionJob{
		ID: document.ID, AgentRunID: document.AgentRunID, ExtractorVersion: document.ExtractorVersion,
		Status: domainmemory.JobStatus(document.Status), Attempts: document.Attempts, LastError: document.LastError,
		CreatedAt: document.CreatedAt, UpdatedAt: document.UpdatedAt, CompletedAt: document.CompletedAt,
	}
}

func DreamRunDocumentFromDomain(run domainmemory.DreamRun) po.DreamRunDocument {
	actions := make([]po.DreamActionDocument, 0, len(run.Actions))
	for _, action := range run.Actions {
		actions = append(actions, po.DreamActionDocument{
			CandidateID: action.CandidateID, MemoryID: action.MemoryID, Decision: string(action.Decision),
			Reason: action.Reason, Applied: action.Applied, FailureReason: action.FailureReason,
		})
	}
	return po.DreamRunDocument{
		ID: run.ID, Status: string(run.Status), Trigger: run.Trigger, CandidateCount: run.CandidateCount,
		CreatedCount: run.CreatedCount, MergedCount: run.MergedCount, SupersededCount: run.SupersededCount,
		RejectedCount: run.RejectedCount, Actions: actions, LastError: run.LastError,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
}

func DreamRunDomainFromDocument(document po.DreamRunDocument) domainmemory.DreamRun {
	actions := make([]domainmemory.DreamAction, 0, len(document.Actions))
	for _, action := range document.Actions {
		actions = append(actions, domainmemory.DreamAction{
			CandidateID: action.CandidateID, MemoryID: action.MemoryID, Decision: domainmemory.DreamDecision(action.Decision),
			Reason: action.Reason, Applied: action.Applied, FailureReason: action.FailureReason,
		})
	}
	return domainmemory.DreamRun{
		ID: document.ID, Status: domainmemory.DreamStatus(document.Status), Trigger: document.Trigger,
		CandidateCount: document.CandidateCount, CreatedCount: document.CreatedCount, MergedCount: document.MergedCount,
		SupersededCount: document.SupersededCount, RejectedCount: document.RejectedCount,
		Actions: actions, LastError: document.LastError, StartedAt: document.StartedAt, FinishedAt: document.FinishedAt,
	}
}

func scopeDocumentFromDomain(scope domainmemory.Scope) po.ScopeDocument {
	return po.ScopeDocument{Type: string(scope.Type), Key: scope.Key}
}

func scopeDomainFromDocument(document po.ScopeDocument) domainmemory.Scope {
	return domainmemory.Scope{Type: domainmemory.ScopeType(document.Type), Key: document.Key}
}

func contentDocumentFromDomain(content domainmemory.Content) po.ContentDocument {
	return po.ContentDocument{
		Goal: content.Goal, ApplicableContext: content.ApplicableContext, Approach: content.Approach,
		Result: content.Result, PainPoints: content.PainPoints, RootCause: content.RootCause,
		Lessons: content.Lessons, Verification: content.Verification,
	}
}

func contentDomainFromDocument(document po.ContentDocument) domainmemory.Content {
	return domainmemory.Content{
		Goal: document.Goal, ApplicableContext: document.ApplicableContext, Approach: document.Approach,
		Result: document.Result, PainPoints: document.PainPoints, RootCause: document.RootCause,
		Lessons: document.Lessons, Verification: document.Verification,
	}
}

func revisionDocumentFromDomain(revision domainmemory.Revision) po.RevisionDocument {
	return po.RevisionDocument{
		ID: revision.ID, Version: revision.Version, Content: contentDocumentFromDomain(revision.Content),
		Confidence: revision.Confidence, Author: string(revision.Author),
		Sources: sourceDocumentsFromDomain(revision.Sources), CreatedAt: revision.CreatedAt,
	}
}

func revisionDomainFromDocument(document po.RevisionDocument) domainmemory.Revision {
	return domainmemory.Revision{
		ID: document.ID, Version: document.Version, Content: contentDomainFromDocument(document.Content),
		Confidence: document.Confidence, Author: domainmemory.AuthorType(document.Author),
		Sources: sourceDomainsFromDocument(document.Sources), CreatedAt: document.CreatedAt,
	}
}

func sourceDocumentsFromDomain(sources []domainmemory.SourceRef) []po.SourceDocument {
	items := make([]po.SourceDocument, 0, len(sources))
	for _, source := range sources {
		items = append(items, po.SourceDocument{
			Type: string(source.Type), SessionID: source.SessionID, AgentRunID: source.AgentRunID,
			EventIDs: append([]string(nil), source.EventIDs...), CreatedAt: source.CreatedAt,
		})
	}
	return items
}

func sourceDomainsFromDocument(sources []po.SourceDocument) []domainmemory.SourceRef {
	items := make([]domainmemory.SourceRef, 0, len(sources))
	for _, source := range sources {
		items = append(items, domainmemory.SourceRef{
			Type: domainmemory.SourceType(source.Type), SessionID: source.SessionID, AgentRunID: source.AgentRunID,
			EventIDs: append([]string(nil), source.EventIDs...), CreatedAt: source.CreatedAt,
		})
	}
	return items
}
