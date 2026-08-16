package agent

import (
	"testing"
	"time"

	domainmemory "myai/core/domain/memory"
)

func TestAIMemoryPayloadMapsCurrentStateAndHistory(t *testing.T) {
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	memory := domainmemory.Memory{
		ID: "memory-1", Title: "Durable extraction", Kind: domainmemory.KindExperience,
		Scope: domainmemory.Scope{Type: domainmemory.ScopeSession, Key: "session-1"},
		Tags:  []string{"go"}, Status: domainmemory.StatusActive, CurrentVersion: 2, UseCount: 4,
		Revisions: []domainmemory.Revision{
			{ID: "revision-1", Version: 1, Content: domainmemory.Content{Goal: "first", Approach: "first"}, Confidence: 0.7, Author: domainmemory.AuthorModel, CreatedAt: now},
			{ID: "revision-2", Version: 2, Content: domainmemory.Content{Goal: "second", Lessons: "second"}, Confidence: 0.9, Author: domainmemory.AuthorHuman, Sources: []domainmemory.SourceRef{{Type: domainmemory.SourceAgentRun, AgentRunID: "run-1", EventIDs: []string{"event-1"}, CreatedAt: now}}, CreatedAt: now.Add(time.Minute)},
		},
		HumanLocked: true, CreatedAt: now, UpdatedAt: now.Add(time.Minute),
	}

	payload := aiMemoryPayload(memory)
	if payload.ID != memory.ID || payload.Scope.Key != "session-1" || payload.CurrentVersion != 2 || payload.UseCount != 4 {
		t.Fatalf("unexpected memory payload: %#v", payload)
	}
	if len(payload.Revisions) != 2 || payload.Revisions[1].Content.Lessons != "second" || payload.Revisions[1].Author != "human" {
		t.Fatalf("revision history was not mapped: %#v", payload.Revisions)
	}
	if len(payload.Revisions[1].Sources) != 1 || payload.Revisions[1].Sources[0].EventIDs[0] != "event-1" {
		t.Fatalf("source references were not mapped: %#v", payload.Revisions[1].Sources)
	}
}

func TestAIMemoryExtractionJobPayloadMapsFailure(t *testing.T) {
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	job := domainmemory.ExtractionJob{
		ID: "job-1", AgentRunID: "run-1", ExtractorVersion: "memory-v1",
		Status: domainmemory.JobFailed, Attempts: 3, LastError: "timeout", CreatedAt: now, UpdatedAt: now,
	}
	payload := aiMemoryExtractionJobsPayload([]domainmemory.ExtractionJob{job}, "failed")
	if len(payload.Jobs) != 1 || payload.Jobs[0].ID != job.ID || payload.Jobs[0].Attempts != 3 || payload.Jobs[0].LastError != "timeout" {
		t.Fatalf("unexpected extraction job payload: %#v", payload)
	}
}
