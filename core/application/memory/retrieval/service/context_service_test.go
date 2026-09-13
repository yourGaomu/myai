package service

import (
	"context"
	"strings"
	"testing"
	"time"

	memoryrepository "myai/core/adapter/persistence/memory/memory"
	memorycatalogcommand "myai/core/application/memory/catalog/command"
	memoryretrievalcommand "myai/core/application/memory/retrieval/command"
	domainmemory "myai/core/domain/memory"
)

func TestContextServiceRanksApplicableScopesAndRecordsUse(t *testing.T) {
	ctx := context.Background()
	store := memoryrepository.New()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	items := []domainmemory.Memory{
		testMemory("global", "Durable extraction retry", domainmemory.KindExperience, domainmemory.Scope{Type: domainmemory.ScopeGlobal}, "Persist jobs before worker submission", now),
		testMemory("workspace", "Extraction failure warning", domainmemory.KindFailure, domainmemory.Scope{Type: domainmemory.ScopeWorkspace, Key: `D:\Go_All\myai`}, "Do not retry failed extraction forever", now.Add(time.Minute)),
		testMemory("other-workspace", "Durable extraction elsewhere", domainmemory.KindExperience, domainmemory.Scope{Type: domainmemory.ScopeWorkspace, Key: `D:\Other`}, "Different project", now.Add(2*time.Minute)),
	}
	for _, item := range items {
		if err := store.Save(ctx, item); err != nil {
			t.Fatalf("save memory %s: %v", item.ID, err)
		}
	}
	usage := &recordingUsage{}
	service := ContextService{
		Memories: store, Usage: usage,
		Scope: domainmemory.Scope{Type: domainmemory.ScopeWorkspace, Key: `D:\Go_All\myai`}, TopK: 4,
	}

	result, err := service.Prepare(ctx, memoryretrievalcommand.Prepare{Input: "durable extraction retry failure", SessionID: "session-1"})
	if err != nil {
		t.Fatalf("prepare memory context: %v", err)
	}
	if !result.Triggered || len(result.MemoryIDs) != 2 {
		t.Fatalf("unexpected retrieval result: %#v", result)
	}
	if contains(result.MemoryIDs, "other-workspace") {
		t.Fatalf("memory from another workspace leaked into result: %#v", result.MemoryIDs)
	}
	if !strings.Contains(result.Prompt, "failure memories as warnings") {
		t.Fatalf("failure safety instruction missing: %s", result.Prompt)
	}
	if len(usage.ids) != len(result.MemoryIDs) {
		t.Fatalf("usage was not recorded for each hit: ids=%#v hits=%#v", usage.ids, result.MemoryIDs)
	}
}

func TestContextServiceSkipsGreeting(t *testing.T) {
	service := ContextService{Memories: memoryrepository.New()}
	result, err := service.Prepare(context.Background(), memoryretrievalcommand.Prepare{Input: "hello"})
	if err != nil {
		t.Fatalf("prepare greeting: %v", err)
	}
	if result.Triggered || result.Prompt != "" {
		t.Fatalf("greeting unexpectedly triggered retrieval: %#v", result)
	}
}

func TestSelectDiverseMemoriesAvoidsRedundantTopK(t *testing.T) {
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	first := testMemory("first", "Retry extraction", domainmemory.KindExperience, domainmemory.Scope{Type: domainmemory.ScopeGlobal}, "persist jobs before retry", now)
	duplicate := testMemory("duplicate", "Retry extraction", domainmemory.KindExperience, domainmemory.Scope{Type: domainmemory.ScopeGlobal}, "persist jobs before retry", now)
	distinct := testMemory("distinct", "Validate schema", domainmemory.KindExperience, domainmemory.Scope{Type: domainmemory.ScopeGlobal}, "validate input before indexing", now)
	selected := selectDiverseMemories([]rankedMemory{
		{memory: first, score: 10}, {memory: duplicate, score: 9.9}, {memory: distinct, score: 9.8},
	}, 2)
	if len(selected) != 2 {
		t.Fatalf("expected two selected memories, got %#v", selected)
	}
	if selected[0].memory.ID != "first" || selected[1].memory.ID != "distinct" {
		t.Fatalf("expected redundant candidate to be replaced by distinct memory, got %#v", selected)
	}
}

type recordingUsage struct{ ids []string }

func (usage *recordingUsage) RecordUse(_ context.Context, command memorycatalogcommand.RecordUse) error {
	usage.ids = append(usage.ids, command.MemoryID)
	return nil
}

func testMemory(id string, title string, kind domainmemory.Kind, scope domainmemory.Scope, approach string, now time.Time) domainmemory.Memory {
	return domainmemory.Memory{
		ID: id, Title: title, Kind: kind, Scope: scope, Status: domainmemory.StatusActive,
		CurrentVersion: 1,
		Revisions: []domainmemory.Revision{{
			ID: id + "-revision", Version: 1,
			Content:    domainmemory.Content{Goal: title, Approach: approach, Lessons: approach},
			Confidence: 0.9, Author: domainmemory.AuthorModel,
			Sources: []domainmemory.SourceRef{{Type: domainmemory.SourceManual, CreatedAt: now}}, CreatedAt: now,
		}},
		CreatedAt: now, UpdatedAt: now,
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
