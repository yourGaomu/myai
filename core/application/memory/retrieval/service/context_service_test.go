package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	memoryrepository "myai/core/adapter/persistence/memory/memory"
	memorycatalogcommand "myai/core/application/memory/catalog/command"
	memoryretrievalcommand "myai/core/application/memory/retrieval/command"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
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

func TestContextServiceHonorsExplicitSearchForShortQuery(t *testing.T) {
	service := ContextService{Memories: memoryrepository.New()}
	result, err := service.Prepare(context.Background(), memoryretrievalcommand.Prepare{Input: "hello"})
	if err != nil {
		t.Fatalf("prepare explicit search: %v", err)
	}
	if !result.Triggered || result.Query != "hello" || result.Prompt != "" {
		t.Fatalf("explicit search was unexpectedly skipped: %#v", result)
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

func TestContextServiceFindsOlderRelevantMemoryBeyondRecentPool(t *testing.T) {
	ctx := context.Background()
	store := memoryrepository.New()
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	old := testMemory("old-relevant", "Quorum recovery", domainmemory.KindExperience, domainmemory.Scope{Type: domainmemory.ScopeGlobal}, "Restore a majority before accepting writes", now.AddDate(-1, 0, 0))
	if err := store.Save(ctx, old); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 205; index++ {
		item := testMemory(fmt.Sprintf("recent-%03d", index), "Palette spacing", domainmemory.KindExperience, domainmemory.Scope{Type: domainmemory.ScopeGlobal}, "Choose neutral colors", now.Add(time.Duration(index)*time.Minute))
		if err := store.Save(ctx, item); err != nil {
			t.Fatal(err)
		}
		if err := store.RecordUse(ctx, item.ID, now.Add(24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	usage := repositoryUsage{store: store, usedAt: now.Add(48 * time.Hour)}
	service := ContextService{Memories: store, Usage: usage, TopK: 1}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := service.Prepare(ctx, memoryretrievalcommand.Prepare{Input: "quorum recovery"})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.MemoryIDs) != 1 || result.MemoryIDs[0] != old.ID {
			t.Fatalf("older relevant memory was crowded out on attempt %d: %#v", attempt, result.MemoryIDs)
		}
	}
	stored, err := store.Get(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.UpdatedAt.Equal(old.UpdatedAt) || stored.UseCount != 2 || stored.LastUsedAt == nil || !stored.LastUsedAt.Equal(usage.usedAt) {
		t.Fatalf("retrieval must update usage without refreshing content: %#v", stored)
	}
}

func TestContextServiceIgnoresLegacyUsageTimestampsWhenChoosingCandidates(t *testing.T) {
	ctx := context.Background()
	store := memoryrepository.New()
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	relevant := testMemory("content-newer", "Quorum recovery", domainmemory.KindExperience, domainmemory.Scope{Type: domainmemory.ScopeGlobal}, "Restore a majority", now.AddDate(-1, 0, 0))
	if err := store.Save(ctx, relevant); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 205; index++ {
		item := testMemory(fmt.Sprintf("frequently-used-%03d", index), relevant.Title, relevant.Kind, relevant.Scope, "Restore a majority", now.AddDate(-2, 0, 0))
		// Older releases stamped updated_at during use. Revision timestamps still
		// tell us when the content actually changed, without a data migration.
		item.UpdatedAt = now.Add(time.Duration(index) * time.Minute)
		item.UseCount = 1000
		item.LastUsedAt = &item.UpdatedAt
		if err := store.Save(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	result, err := (ContextService{Memories: store, TopK: 1}).Prepare(ctx, memoryretrievalcommand.Prepare{Input: "quorum recovery"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MemoryIDs) != 1 || result.MemoryIDs[0] != relevant.ID {
		t.Fatalf("legacy usage timestamps displaced fresher content: %#v", result.MemoryIDs)
	}
}

func TestContextServiceMatchesWordsWithoutSubstringNoise(t *testing.T) {
	cases := []struct {
		name  string
		query string
		title string
		tags  []string
		want  bool
	}{
		{name: "english word", query: "recover REDIS safely", title: "Redis", want: true},
		{name: "punctuation boundary", query: "go/retry", title: "Worker strategy", tags: []string{"go"}, want: true},
		{name: "chinese phrase", query: "如何恢复数据库连接", title: "数据库连接恢复", want: true},
		{name: "mixed languages", query: "Go语言重试", title: "Go retry", want: true},
		{name: "unrelated", query: "quorum", title: "Palette spacing", want: false},
		{name: "latin bigram", query: "redis", title: "Predictable layout", want: false},
		{name: "tag substring", query: "cargo", title: "Worker strategy", tags: []string{"go"}, want: false},
		{name: "word suffix", query: "sql", title: "MySQL", want: false},
		{name: "short chinese overlap", query: "内存", title: "内容存档", want: false},
		{name: "empty tag", query: "quorum", title: "Palette spacing", tags: []string{"", " "}, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store := memoryrepository.New()
			item := testMemory("candidate", test.title, domainmemory.KindExperience, domainmemory.Scope{Type: domainmemory.ScopeGlobal}, "Keep evidence", time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC))
			item.Tags = test.tags
			if err := store.Save(ctx, item); err != nil {
				t.Fatal(err)
			}
			result, err := (ContextService{Memories: store}).Prepare(ctx, memoryretrievalcommand.Prepare{Input: test.query})
			if err != nil {
				t.Fatal(err)
			}
			if got := len(result.MemoryIDs) == 1; got != test.want {
				t.Fatalf("query %q against %q tags %q: recalled=%v, want %v", test.query, test.title, test.tags, got, test.want)
			}
		})
	}
}

func TestContextServiceDoesNotBoostSubstringOrEmptyTags(t *testing.T) {
	ctx := context.Background()
	store := memoryrepository.New()
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	for _, id := range []string{"weak", "exact"} {
		item := testMemory(id, "Cache operations", domainmemory.KindExperience, domainmemory.Scope{Type: domainmemory.ScopeGlobal}, "quorum", now)
		if id == "weak" {
			item.Tags = []string{"or", "", "内", "存"}
		} else {
			item.Tags = []string{"quorum"}
		}
		if err := store.Save(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	result, err := (ContextService{Memories: store, TopK: 1}).Prepare(ctx, memoryretrievalcommand.Prepare{Input: "quorum"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MemoryIDs) != 1 || result.MemoryIDs[0] != "exact" {
		t.Fatalf("weak tags outranked an exact tag: %#v", result.MemoryIDs)
	}
}

func TestContextServiceRequestsFilteredContentOrderedCandidates(t *testing.T) {
	store := &capturingMemories{Repository: memoryrepository.New()}
	_, err := (ContextService{Memories: store}).Prepare(context.Background(), memoryretrievalcommand.Prepare{Input: "Redis 数据库"})
	if err != nil {
		t.Fatal(err)
	}
	filter := store.filter
	if filter.Limit != 200 || !filter.OrderByContent || len(filter.Statuses) != 1 || filter.Statuses[0] != domainmemory.StatusActive {
		t.Fatalf("unexpected recall candidate request: %#v", filter)
	}
	if !contains(filter.SearchTerms, "redis") || !contains(filter.SearchTerms, "数据库") {
		t.Fatalf("candidate query must filter relevance before limiting: %#v", filter)
	}
}

type capturingMemories struct {
	memoryport.Repository
	filter memoryport.ListFilter
}

func (store *capturingMemories) List(ctx context.Context, filter memoryport.ListFilter) ([]domainmemory.Memory, error) {
	store.filter = filter
	return store.Repository.List(ctx, filter)
}

type repositoryUsage struct {
	store  memoryport.UsageRepository
	usedAt time.Time
}

func (usage repositoryUsage) RecordUse(ctx context.Context, command memorycatalogcommand.RecordUse) error {
	return usage.store.RecordUse(ctx, command.MemoryID, usage.usedAt)
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
