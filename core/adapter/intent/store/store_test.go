package store

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	intentport "myai/core/port/intent"
)

func TestIntentConfigBSONRoundTrip(t *testing.T) {
	original := configDocumentFromDomain(intentport.Config{
		Strategy: intentport.StrategyJev, BaseURL: "https://api.typesafe.ai", APIKey: "secret",
		Model: "jev-latest", PlanConfidence: 0.55, ExecuteConfidence: 0.85,
	})
	encoded, err := bson.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded configDocument
	if err := bson.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != "default" || decoded.domain() != original.domain() {
		t.Fatalf("BSON round trip changed config: %#v", decoded)
	}
}

func TestIntentTraceBSONRoundTrip(t *testing.T) {
	original := intentport.Trace{ID: "trace-1", SessionID: "session-1", RequestID: "request-1", CreatedAt: time.Now().UTC().Truncate(time.Millisecond), ExpiresAt: time.Now().UTC().Truncate(time.Millisecond).Add(time.Hour), RequestBody: `{"state":"input"}`, ResponseBody: `{"choice":"implementation"}`, Status: "succeeded", Choice: "implementation", Confidence: 0.9, ShouldPlan: true, ShouldExecute: true}
	encoded, err := bson.Marshal(traceDocumentFromDomain(original))
	if err != nil {
		t.Fatal(err)
	}
	var decoded traceDocument
	if err := bson.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.domain(); got != original {
		t.Fatalf("BSON round trip changed trace: %#v", got)
	}
}

func TestMemoryIntentStoreFiltersAndExpiresTraces(t *testing.T) {
	store := NewMemory()
	now := time.Now()
	for _, trace := range []intentport.Trace{
		{ID: "one", SessionID: "a", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RequestBody: "request", ResponseBody: "response"},
		{ID: "two", SessionID: "b", CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)},
		{ID: "expired", SessionID: "a", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Second)},
	} {
		if err := store.SaveTrace(context.Background(), trace); err != nil {
			t.Fatal(err)
		}
	}
	store.lastPrune = time.Now().Add(-2 * time.Hour)
	if err := store.SaveTrace(context.Background(), intentport.Trace{ID: "new", SessionID: "b", CreatedAt: now.Add(time.Minute), ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListTraces(context.Background(), "a", 10)
	if err != nil || len(items) != 1 || items[0].ID != "one" || items[0].RequestBody != "" {
		t.Fatalf("filtered summaries = %#v, %v", items, err)
	}
	if _, exists := store.traces["expired"]; exists {
		t.Fatal("expired in-memory trace was not pruned")
	}
	detail, err := store.GetTrace(context.Background(), "one")
	if err != nil || detail.RequestBody != "request" {
		t.Fatalf("detail = %#v, %v", detail, err)
	}
	if err := store.ClearTraces(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetTrace(context.Background(), "one"); err != intentport.ErrNotFound {
		t.Fatalf("cleared trace error = %v", err)
	}
}
