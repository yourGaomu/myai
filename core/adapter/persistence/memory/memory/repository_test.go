package memory

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
)

func TestListLimitMatchesMongoContract(t *testing.T) {
	ctx := context.Background()
	store := New()
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	for index := 0; index < 205; index++ {
		item := storedMemory(fmt.Sprintf("memory-%03d", index), now.Add(time.Duration(index)*time.Minute))
		if err := store.Save(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		limit int
		want  int
	}{
		{limit: -1, want: 50}, {limit: 0, want: 50}, {limit: 1, want: 1},
		{limit: 100, want: 100}, {limit: 101, want: 101}, {limit: 200, want: 200},
		{limit: 201, want: 200}, {limit: 10000, want: 200},
	} {
		items, err := store.List(ctx, memoryport.ListFilter{Limit: test.limit})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != test.want || items[0].ID != "memory-204" {
			t.Fatalf("limit %d returned %d memories with wrong size/order; want %d", test.limit, len(items), test.want)
		}
	}
}

func TestRecordUsePreservesContentAndContentOrder(t *testing.T) {
	ctx := context.Background()
	store := New()
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	older := storedMemory("older", now.AddDate(-2, 0, 0))
	newer := storedMemory("newer", now.AddDate(-1, 0, 0))
	for _, item := range []domainmemory.Memory{older, newer} {
		if err := store.Save(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := store.RecordUse(ctx, older.ID, now); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := store.Get(ctx, older.ID)
	if err != nil {
		t.Fatal(err)
	}
	expected := older
	expected.UseCount, expected.LastUsedAt = 3, &now
	if !reflect.DeepEqual(stored, expected) {
		t.Fatalf("usage changed content or lost counters: want %#v, got %#v", expected, stored)
	}
	for _, orderByContent := range []bool{false, true} {
		items, err := store.List(ctx, memoryport.ListFilter{OrderByContent: orderByContent, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != newer.ID {
			t.Fatalf("usage displaced newer content (content order=%v): %#v", orderByContent, items)
		}
	}
}

func TestListSearchesOnlyCurrentContent(t *testing.T) {
	ctx := context.Background()
	store := New()
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	item := storedMemory("revised", now.AddDate(-1, 0, 0))
	item.Revisions[0].Content = domainmemory.Content{Goal: "Quorum recovery", Approach: "Restore a majority"}
	if err := item.AppendRevision(domainmemory.Revision{
		ID: "current", Content: domainmemory.Content{Goal: "Palette spacing", Approach: "Choose neutral colors"},
		Confidence: 0.9, Author: domainmemory.AuthorHuman, CreatedAt: now,
	}, now); err != nil {
		t.Fatal(err)
	}
	// Persistence order is not a version contract: the current revision can
	// precede retired content in an existing document.
	item.Revisions[0], item.Revisions[1] = item.Revisions[1], item.Revisions[0]
	if err := store.Save(ctx, item); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		term string
		want int
	}{
		{term: "quorum", want: 0}, {term: "palette", want: 1},
	} {
		items, err := store.List(ctx, memoryport.ListFilter{SearchTerms: []string{test.term}, OrderByContent: true, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != test.want {
			t.Fatalf("search for %q did not use current_version: %#v", test.term, items)
		}
	}
}

func storedMemory(id string, now time.Time) domainmemory.Memory {
	return domainmemory.Memory{
		ID: id, Title: "Memory", Kind: domainmemory.KindExperience,
		Scope: domainmemory.Scope{Type: domainmemory.ScopeGlobal}, Status: domainmemory.StatusActive,
		CurrentVersion: 1,
		Revisions: []domainmemory.Revision{{
			ID: id + "-revision", Version: 1,
			Content:    domainmemory.Content{Goal: "Keep evidence", Approach: "Validate before saving"},
			Confidence: 0.9, Author: domainmemory.AuthorHuman, CreatedAt: now,
		}},
		CreatedAt: now, UpdatedAt: now,
	}
}
