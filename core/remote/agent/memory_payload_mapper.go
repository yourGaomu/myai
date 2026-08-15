package agent

import (
	domainmemory "myai/core/domain/memory"
	"myai/core/remote/protocol"
)

func aiMemoryPayload(memory domainmemory.Memory) protocol.AIMemory {
	revisions := make([]protocol.AIMemoryRevision, 0, len(memory.Revisions))
	for _, revision := range memory.Revisions {
		revisions = append(revisions, protocol.AIMemoryRevision{
			ID: revision.ID, Version: revision.Version, Content: aiMemoryContentPayload(revision.Content),
			Confidence: revision.Confidence, Author: string(revision.Author),
			Sources: aiMemorySourcesPayload(revision.Sources), CreatedAt: revision.CreatedAt,
		})
	}
	return protocol.AIMemory{
		ID: memory.ID, Title: memory.Title, Kind: string(memory.Kind),
		Scope: protocol.AIMemoryScope{Type: string(memory.Scope.Type), Key: memory.Scope.Key},
		Tags:  append([]string(nil), memory.Tags...), Status: string(memory.Status), CurrentVersion: memory.CurrentVersion,
		Revisions: revisions, HumanLocked: memory.HumanLocked, SupersedesID: memory.SupersedesID,
		UseCount: memory.UseCount, LastUsedAt: memory.LastUsedAt, DeletedAt: memory.DeletedAt,
		DeletionReason: memory.DeletionReason, CreatedAt: memory.CreatedAt, UpdatedAt: memory.UpdatedAt,
	}
}

func aiMemoryCandidatePayload(candidate domainmemory.Candidate) protocol.AIMemoryCandidate {
	return protocol.AIMemoryCandidate{
		ID: candidate.ID, Title: candidate.Title, Kind: string(candidate.Kind),
		Scope: protocol.AIMemoryScope{Type: string(candidate.Scope.Type), Key: candidate.Scope.Key},
		Tags:  append([]string(nil), candidate.Tags...), Content: aiMemoryContentPayload(candidate.Content),
		Confidence: candidate.Confidence, Sources: aiMemorySourcesPayload(candidate.Sources),
		Status: string(candidate.Status), TargetMemoryID: candidate.TargetMemoryID, ReviewNote: candidate.ReviewNote,
		CreatedAt: candidate.CreatedAt, UpdatedAt: candidate.UpdatedAt,
	}
}

func aiMemoryContentPayload(content domainmemory.Content) protocol.AIMemoryContent {
	return protocol.AIMemoryContent{
		Goal: content.Goal, ApplicableContext: content.ApplicableContext, Approach: content.Approach,
		Result: content.Result, PainPoints: content.PainPoints, RootCause: content.RootCause,
		Lessons: content.Lessons, Verification: content.Verification,
	}
}

func aiMemorySourcesPayload(sources []domainmemory.SourceRef) []protocol.AIMemorySource {
	items := make([]protocol.AIMemorySource, 0, len(sources))
	for _, source := range sources {
		items = append(items, protocol.AIMemorySource{
			Type: string(source.Type), SessionID: source.SessionID, AgentRunID: source.AgentRunID,
			EventIDs: append([]string(nil), source.EventIDs...), CreatedAt: source.CreatedAt,
		})
	}
	return items
}

func aiMemoriesPayload(memories []domainmemory.Memory, message string) protocol.AIMemoryListResultPayload {
	items := make([]protocol.AIMemory, 0, len(memories))
	for _, memory := range memories {
		items = append(items, aiMemoryPayload(memory))
	}
	return protocol.AIMemoryListResultPayload{Memories: items, Message: message}
}

func aiMemoryCandidatesPayload(candidates []domainmemory.Candidate, message string) protocol.AIMemoryCandidateListResultPayload {
	items := make([]protocol.AIMemoryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, aiMemoryCandidatePayload(candidate))
	}
	return protocol.AIMemoryCandidateListResultPayload{Candidates: items, Message: message}
}
