package mapper

import (
	"reflect"
	"testing"
	"time"

	domainmemory "myai/core/domain/memory"
)

func TestMemoryDocumentRoundTrip(t *testing.T) {
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	lastUsedAt := now.Add(time.Minute)
	deletedAt := now.Add(2 * time.Minute)
	memory := domainmemory.Memory{
		ID: "memory-1", Title: "Durable extraction", Kind: domainmemory.KindExperience,
		Scope: domainmemory.Scope{Type: domainmemory.ScopeWorkspace, Key: `D:\Go_All\myai`},
		Tags:  []string{"go", "memory"}, Status: domainmemory.StatusDeleted, CurrentVersion: 1,
		Revisions: []domainmemory.Revision{{
			ID: "revision-1", Version: 1,
			Content:    domainmemory.Content{Goal: "Recover extraction", Approach: "Persist jobs", Verification: "restart test"},
			Confidence: 0.9, Author: domainmemory.AuthorHuman,
			Sources:   []domainmemory.SourceRef{{Type: domainmemory.SourceAgentRun, SessionID: "session-1", AgentRunID: "run-1", EventIDs: []string{"event-1"}, CreatedAt: now}},
			CreatedAt: now,
		}},
		HumanLocked: true, SupersedesID: "memory-0", UseCount: 3, LastUsedAt: &lastUsedAt,
		DeletedAt: &deletedAt, DeletionReason: "obsolete", CreatedAt: now, UpdatedAt: deletedAt,
	}

	roundTrip := MemoryDomainFromDocument(MemoryDocumentFromDomain(memory))
	if !reflect.DeepEqual(roundTrip, memory) {
		t.Fatalf("memory mapper round trip mismatch:\nwant %#v\n got %#v", memory, roundTrip)
	}
}

func TestCandidateAndExtractionJobDocumentRoundTrip(t *testing.T) {
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	candidate := domainmemory.Candidate{
		ID: "candidate-1", OriginKey: "job-1:0", Title: "Retry warning", Kind: domainmemory.KindFailure,
		Scope: domainmemory.Scope{Type: domainmemory.ScopeGlobal}, Tags: []string{"retry"},
		Content: domainmemory.Content{Goal: "Avoid loops", Lessons: "Stop after three attempts"}, Confidence: 0.8,
		Sources: []domainmemory.SourceRef{{Type: domainmemory.SourceAgentRun, AgentRunID: "run-1", CreatedAt: now}},
		Status:  domainmemory.CandidateRejected, TargetMemoryID: "memory-1", ReviewNote: "duplicate",
		CreatedAt: now, UpdatedAt: now.Add(time.Minute),
	}
	if roundTrip := CandidateDomainFromDocument(CandidateDocumentFromDomain(candidate)); !reflect.DeepEqual(roundTrip, candidate) {
		t.Fatalf("candidate mapper round trip mismatch:\nwant %#v\n got %#v", candidate, roundTrip)
	}

	completedAt := now.Add(2 * time.Minute)
	job := domainmemory.ExtractionJob{
		ID: "job-1", AgentRunID: "run-1", ExtractorVersion: "model-memory-v1",
		Status: domainmemory.JobSucceeded, Attempts: 2, CreatedAt: now, UpdatedAt: completedAt, CompletedAt: &completedAt,
	}
	if roundTrip := ExtractionJobDomainFromDocument(ExtractionJobDocumentFromDomain(job)); !reflect.DeepEqual(roundTrip, job) {
		t.Fatalf("job mapper round trip mismatch:\nwant %#v\n got %#v", job, roundTrip)
	}
}
