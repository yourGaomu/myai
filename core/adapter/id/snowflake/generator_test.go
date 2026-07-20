package snowflake

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestNewRejectsInvalidNodeID(t *testing.T) {
	if _, err := New(-1); err == nil {
		t.Fatal("expected negative node id to fail")
	}
	if _, err := New(maxNodeID + 1); err == nil {
		t.Fatal("expected oversized node id to fail")
	}
}

func TestGeneratorProducesUniqueIDsWithinSameMillisecond(t *testing.T) {
	now := time.Date(2026, time.July, 19, 0, 0, 0, 0, time.UTC)
	generator, err := newWithClock(7, defaultEpoch, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]struct{}, 5000)
	for range 5000 {
		id := generator.NewID()
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate snowflake id %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestGeneratorRemainsUniqueWhenClockMovesBackward(t *testing.T) {
	current := time.Date(2026, time.July, 19, 0, 0, 1, 0, time.UTC)
	generator, err := newWithClock(1, defaultEpoch, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}

	first := parseID(t, generator.NewID())
	current = current.Add(-time.Second)
	second := parseID(t, generator.NewID())
	if second <= first {
		t.Fatalf("snowflake ids must stay increasing after clock rollback: %d <= %d", second, first)
	}
}

func TestGeneratorIsSafeForConcurrentUse(t *testing.T) {
	generator, err := New(3)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 16
	const perWorker = 500
	ids := make(chan string, workers*perWorker)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range perWorker {
				ids <- generator.NewID()
			}
		}()
	}
	wait.Wait()
	close(ids)

	seen := make(map[string]struct{}, workers*perWorker)
	for id := range ids {
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate snowflake id %s", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != workers*perWorker {
		t.Fatalf("got %d ids, want %d", len(seen), workers*perWorker)
	}
}

func parseID(t *testing.T, value string) int64 {
	t.Helper()
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("parse snowflake id %q: %v", value, err)
	}
	return id
}
