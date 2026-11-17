package repository

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	mongomapper "myai/core/adapter/persistence/mongo/mapper"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	generation "myai/core/domain/generation"
	agentplan "myai/core/plan"
	portrepository "myai/core/port/repository"
	"myai/core/session"
)

func TestTouchSessionMessagesPreservesFieldsOwnedByOtherWriters(t *testing.T) {
	ops := newMetadataMongo(t)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	temperature := 0.7
	current := portrepository.SessionRecord{
		ID: "session", Model: "chosen-model", AgentMode: string(session.AgentModePlan),
		PermissionMode: "ask", Title: "User named this", ContextWindowK: 128,
		Summary: "new compaction", CompactedMessages: 9,
		GenerationSettings: generation.Settings{Temperature: &temperature},
		StyleInstruction:   "custom style", CurrentPlan: metadataPlan("current", 5),
		Usage: metadataUsage(100), LastUsage: metadataUsage(30),
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
	ops.seed(current)
	ops.documents[current.ID]["message_usage_sequence"] = int64(50)
	before := metadataBSON(t, ops.documents[current.ID])
	stale := portrepository.SessionRecord{
		ID: current.ID, Model: "old-model", AgentMode: string(session.AgentModeChat),
		PermissionMode: "auto", Title: "Generated old title", ContextWindowK: 16,
		Summary: "old compaction", CompactedMessages: 1, CurrentPlan: metadataPlan("old", 1),
		Usage: metadataUsage(20), LastUsage: metadataUsage(5), UpdatedAt: now.Add(-time.Minute),
	}
	if err := NewWithTemplate(ops).TouchSessionMessages(context.Background(), stale, 49, true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, ops.documents[current.ID]) {
		t.Fatalf("stale snapshot changed persisted state: before=%#v after=%#v", before, ops.documents[current.ID])
	}
	for _, call := range ops.updates {
		if call.upsert && !reflect.DeepEqual(call.filter, bson.M{"_id": current.ID}) {
			t.Fatalf("conditional upsert could duplicate an existing session: %#v", call)
		}
		for operator, raw := range call.update {
			if operator == "$setOnInsert" {
				continue
			}
			for key := range raw.(bson.M) {
				if key != "updated_at" && key != "title" && key != "last_usage" && key != "message_usage_sequence" && key != "usage" && !strings.HasPrefix(key, "usage.") {
					t.Fatalf("message writer owns forbidden field %s: %#v", key, call)
				}
			}
		}
	}
	if ops.updates[len(ops.updates)-1].result.MatchedCount != 0 {
		t.Fatal("older usage snapshot should not match")
	}
}

func TestTouchSessionMessagesInitializesWithoutPlanOrUnrequestedUsage(t *testing.T) {
	ops := newMetadataMongo(t)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	record := portrepository.SessionRecord{
		ID: "session", Kind: "user", Model: "model", AgentMode: string(session.AgentModePlan),
		PermissionMode: "ask", Title: "Initial title", ContextWindowK: 64,
		CurrentPlan: metadataPlan("stale", 9), Usage: metadataUsage(400), LastUsage: metadataUsage(20),
		CreatedAt: now, UpdatedAt: now,
	}
	store := NewWithTemplate(ops)
	for attempt := 0; attempt < 2; attempt++ {
		if err := store.TouchSessionMessages(context.Background(), record, 0, false); err != nil {
			t.Fatal(err)
		}
	}
	document := ops.documents[record.ID]
	for _, key := range []string{"current_plan", "usage", "last_usage", "message_usage_sequence"} {
		if _, exists := document[key]; exists {
			t.Fatalf("initial message snapshot inserted %q: %#v", key, document[key])
		}
	}
	if document["agent_mode"] != string(session.AgentModeChat) || document["model"] != record.Model || document["title"] != record.Title || document["permission_mode"] != record.PermissionMode {
		t.Fatalf("unexpected initial defaults: %#v", document)
	}
	if len(ops.documents) != 1 || ops.updates[0].result.UpsertedCount != 1 || ops.updates[2].result.MatchedCount != 1 {
		t.Fatalf("initialization did not match on retry: %#v", ops.updates)
	}
}

func TestTouchSessionMessagesTitleOnlyReplacesDefault(t *testing.T) {
	for _, title := range []any{nil, "", "New chat", "User named this"} {
		t.Run(fmt.Sprint(title), func(t *testing.T) {
			ops := newMetadataMongo(t)
			ops.documents["session"] = bson.M{"_id": "session"}
			if title != nil {
				ops.documents["session"]["title"] = title
			}
			store := NewWithTemplate(ops)
			if err := store.TouchSessionMessages(context.Background(), portrepository.SessionRecord{ID: "session", Title: "Generated title"}, 0, false); err != nil {
				t.Fatal(err)
			}
			want := "Generated title"
			if title == "User named this" {
				want = title.(string)
			}
			if got := ops.documents["session"]["title"]; got != want {
				t.Fatalf("title=%v, want %v", got, want)
			}
		})
	}
}

func TestTouchSessionMessagesUsageIsOrderedCumulativeAndRetrySafe(t *testing.T) {
	ops := newMetadataMongo(t)
	store := NewWithTemplate(ops)
	ctx := context.Background()
	record := portrepository.SessionRecord{ID: "session", Usage: metadataUsage(100), LastUsage: metadataUsage(20)}
	if err := store.TouchSessionMessages(ctx, record, 10, true); err != nil {
		t.Fatal(err)
	}
	committed := metadataBSON(t, ops.documents[record.ID])
	// Even a different payload with the same sequence cannot be applied twice.
	for _, sequence := range []int64{10, 9} {
		retry := record
		retry.Usage, retry.LastUsage = metadataUsage(1000), metadataUsage(900)
		if err := NewWithTemplate(ops).TouchSessionMessages(ctx, retry, sequence, true); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(committed, ops.documents[record.ID]) {
			t.Fatalf("retry/older usage applied at sequence %d: %#v", sequence, ops.documents[record.ID])
		}
	}
	record.Usage, record.LastUsage = metadataUsage(40), metadataUsage(4)
	if err := store.TouchSessionMessages(ctx, record, 11, true); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Usage, metadataUsage(100)) || !reflect.DeepEqual(got.LastUsage, metadataUsage(4)) {
		t.Fatalf("cumulative usage regressed or last usage did not advance: %#v %#v", got.Usage, got.LastUsage)
	}
	if ops.documents[record.ID]["message_usage_sequence"] != int64(11) {
		t.Fatal("new usage sequence was not committed")
	}
	for _, call := range ops.updates {
		if _, exists := call.update["$inc"]; exists {
			t.Fatal("usage snapshots must not increment totals on retry")
		}
	}
	if ops.messageInserts != 0 {
		t.Fatal("metadata retries inserted messages")
	}
}

func TestTouchSessionMessagesLegacyUsageAndMissingSnapshot(t *testing.T) {
	for _, legacy := range []string{"missing", "null", "cumulative"} {
		t.Run(legacy, func(t *testing.T) {
			ops := newMetadataMongo(t)
			ops.documents["session"] = bson.M{"_id": "session", "title": "New chat"}
			if legacy == "null" {
				ops.documents["session"]["usage"] = nil
			} else if legacy == "cumulative" {
				ops.seed(portrepository.SessionRecord{ID: "session", Usage: metadataUsage(200)})
			}
			store := NewWithTemplate(ops)
			record := portrepository.SessionRecord{ID: "session", Usage: metadataUsage(50), LastUsage: metadataUsage(10)}
			if err := store.TouchSessionMessages(context.Background(), record, 1, true); err != nil {
				t.Fatal(err)
			}
			want := metadataUsage(50)
			if legacy == "cumulative" {
				want = metadataUsage(200)
			}
			got, err := store.GetSession(context.Background(), record.ID)
			if err != nil || !reflect.DeepEqual(got.Usage, want) {
				t.Fatalf("legacy usage=%#v err=%v, want %#v", got.Usage, err, want)
			}
			before := metadataBSON(t, ops.documents[record.ID])
			if err := store.TouchSessionMessages(context.Background(), portrepository.SessionRecord{ID: record.ID}, 2, true); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, ops.documents[record.ID]) {
				t.Fatal("absent usage erased totals or advanced the usage fence")
			}
		})
	}
}

func TestTouchSessionMessagesRejectsUnorderedUsageBeforeWriting(t *testing.T) {
	for _, sequence := range []int64{0, -1} {
		ops := newMetadataMongo(t)
		err := NewWithTemplate(ops).TouchSessionMessages(context.Background(), portrepository.SessionRecord{ID: "session", Usage: metadataUsage(1)}, sequence, true)
		if err == nil || len(ops.updates) != 0 {
			t.Fatalf("invalid sequence %d wrote metadata: err=%v updates=%#v", sequence, err, ops.updates)
		}
	}
}

func TestTouchSessionMessagesRetriesAfterPartialCommit(t *testing.T) {
	ops := newMetadataMongo(t)
	store := NewWithTemplate(ops)
	record := portrepository.SessionRecord{ID: "session", Title: "Title", Usage: metadataUsage(100), LastUsage: metadataUsage(20)}
	transport := errors.New("transport failed after commit")
	ops.afterCommit = func(call metadataMongoCall) error {
		if set, ok := call.update["$set"].(bson.M); ok {
			if _, exists := set["message_usage_sequence"]; exists {
				return transport
			}
		}
		return nil
	}
	if err := store.TouchSessionMessages(context.Background(), record, 1, true); !errors.Is(err, transport) {
		t.Fatalf("expected transport failure, got %v", err)
	}
	ops.afterCommit = nil
	committed := metadataBSON(t, ops.documents[record.ID])
	if err := store.TouchSessionMessages(context.Background(), record, 1, true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(committed, ops.documents[record.ID]) || len(ops.documents) != 1 || ops.messageInserts != 0 {
		t.Fatal("retry after transport error duplicated state")
	}
}

func TestSaveSessionPlanUsesIdentityRevisionCASWithoutUpsert(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprint("legacy=", legacy), func(t *testing.T) {
			ops := newMetadataMongo(t)
			revision := int64(4)
			if legacy {
				revision = 0
			}
			ops.seed(portrepository.SessionRecord{ID: "session", CurrentPlan: metadataPlan("current", revision), Title: "Chosen title", AgentMode: "plan"})
			store := NewWithTemplate(ops)
			next := metadataPlan("current", revision+1)
			next.Status = agentplan.StatusRunning
			if err := store.SaveSessionPlan(context.Background(), "session", "current", revision, next); err != nil {
				t.Fatal(err)
			}
			if len(ops.updates) != 1 || ops.updates[0].upsert || ops.updates[0].result.MatchedCount != 1 {
				t.Fatalf("unexpected CAS operation: %#v", ops.updates)
			}
			got, err := store.GetSession(context.Background(), "session")
			if err != nil || got.CurrentPlan.Status != next.Status || got.CurrentPlan.Revision != next.Revision || got.Title != "Chosen title" || got.AgentMode != "plan" {
				t.Fatalf("CAS touched other session fields or failed: %#v err=%v", got, err)
			}
		})
	}
}

func TestSaveSessionPlanRejectsStalePlanAndMissingSession(t *testing.T) {
	for _, state := range []string{"other-id", "newer-revision", "missing-session"} {
		t.Run(state, func(t *testing.T) {
			ops := newMetadataMongo(t)
			if state != "missing-session" {
				current := metadataPlan("old", 4)
				if state == "other-id" {
					current = metadataPlan("new", 1)
				}
				ops.seed(portrepository.SessionRecord{ID: "session", CurrentPlan: current})
			}
			before := metadataBSON(t, bson.M{"documents": ops.documents})
			err := NewWithTemplate(ops).SaveSessionPlan(context.Background(), "session", "old", 2, metadataPlan("old", 3))
			if !errors.Is(err, portrepository.ErrConflict) {
				t.Fatalf("expected conflict, got %v", err)
			}
			if !reflect.DeepEqual(before, metadataBSON(t, bson.M{"documents": ops.documents})) || ops.updates[0].upsert || ops.updates[0].result.MatchedCount != 0 {
				t.Fatal("stale plan resurrected a plan or session")
			}
		})
	}
}

func TestSaveSessionPlanCreatesOnlyWhenPlanIsEmpty(t *testing.T) {
	for _, state := range []string{"missing-field", "null", "existing-plan"} {
		t.Run(state, func(t *testing.T) {
			ops := newMetadataMongo(t)
			ops.documents["session"] = bson.M{"_id": "session"}
			if state == "null" {
				ops.documents["session"]["current_plan"] = nil
			} else if state == "existing-plan" {
				ops.seed(portrepository.SessionRecord{ID: "session", CurrentPlan: metadataPlan("other", 2)})
			}
			err := NewWithTemplate(ops).SaveSessionPlan(context.Background(), "session", "", 0, metadataPlan("new", 1))
			if state == "existing-plan" {
				if !errors.Is(err, portrepository.ErrConflict) {
					t.Fatalf("creation overwrote existing plan: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if ops.updates[0].upsert {
				t.Fatal("plan creation upserted the session")
			}
		})
	}
}

func TestSaveSessionPlanRetryRequiresIdenticalPersistedPayload(t *testing.T) {
	ops := newMetadataMongo(t)
	store := NewWithTemplate(ops)
	ops.seed(portrepository.SessionRecord{ID: "session", CurrentPlan: metadataPlan("plan", 1)})
	next := metadataPlan("plan", 2)
	// Mongo truncates timestamps to milliseconds; the retry compares stored BSON.
	next.UpdatedAt = next.UpdatedAt.Add(123456 * time.Nanosecond)
	transport := errors.New("connection lost after commit")
	ops.afterCommit = func(metadataMongoCall) error { return transport }
	if err := store.SaveSessionPlan(context.Background(), "session", "plan", 1, next); !errors.Is(err, transport) {
		t.Fatalf("expected transport error, got %v", err)
	}
	ops.afterCommit = nil
	if err := NewWithTemplate(ops).SaveSessionPlan(context.Background(), "session", "plan", 1, next); err != nil {
		t.Fatalf("identical retry failed: %v", err)
	}
	if ops.updates[1].result.MatchedCount != 0 {
		t.Fatal("retry unexpectedly matched old revision")
	}
	other := agentplan.Clone(next)
	other.Status, other.Steps[0].Status = agentplan.StatusDone, agentplan.StepStatusDone
	if err := store.SaveSessionPlan(context.Background(), "session", "plan", 1, other); !errors.Is(err, portrepository.ErrConflict) {
		t.Fatalf("different payload with same identity/revision accepted: %v", err)
	}
}

func TestSaveSessionPlanExplicitReplacementRequiresExpectedPlan(t *testing.T) {
	ops := newMetadataMongo(t)
	ops.seed(portrepository.SessionRecord{ID: "session", CurrentPlan: metadataPlan("old", 4)})
	store := NewWithTemplate(ops)
	next := metadataPlan("new", 1)
	if err := store.SaveSessionPlan(context.Background(), "session", "old", 4, next); err != nil {
		t.Fatalf("explicit replacement failed: %v", err)
	}
	if err := store.SaveSessionPlan(context.Background(), "session", "old", 4, metadataPlan("old", 5)); !errors.Is(err, portrepository.ErrConflict) {
		t.Fatalf("late old plan resurrected after replacement: %v", err)
	}
	if err := store.SaveSessionPlan(context.Background(), "session", "old", 4, next); err != nil {
		t.Fatalf("replacement retry failed: %v", err)
	}
	if ops.documents["session"]["current_plan"].(bson.M)["id"] != "new" {
		t.Fatal("old plan replaced current identity")
	}
	for _, call := range ops.updates {
		if call.upsert {
			t.Fatal("replacement must never upsert")
		}
	}
}

func TestSaveSessionPlanClearIsRetrySafe(t *testing.T) {
	ops := newMetadataMongo(t)
	ops.seed(portrepository.SessionRecord{ID: "session", CurrentPlan: metadataPlan("plan", 1)})
	store := NewWithTemplate(ops)
	for attempt := 0; attempt < 2; attempt++ {
		if err := store.SaveSessionPlan(context.Background(), "session", "plan", 1, nil); err != nil {
			t.Fatal(err)
		}
	}
	if ops.updates[1].result.MatchedCount != 0 || ops.documents["session"]["current_plan"] != nil {
		t.Fatal("clearing a plan was not idempotent")
	}
}

func TestSaveSessionPlanRejectsInvalidTransitionsBeforeWriting(t *testing.T) {
	cases := []struct {
		name     string
		expected string
		revision int64
		next     *agentplan.Plan
	}{
		{name: "old-id-resurrection", expected: "new", revision: 2, next: metadataPlan("old", 3)},
		{name: "revision-regression", expected: "plan", revision: 2, next: metadataPlan("plan", 1)},
		{name: "same-revision", expected: "plan", revision: 2, next: metadataPlan("plan", 2)},
		{name: "skipped-revision", expected: "plan", revision: 2, next: metadataPlan("plan", 4)},
		{name: "stale-creation", next: metadataPlan("plan", 4)},
		{name: "negative-expected", expected: "plan", revision: -1, next: metadataPlan("plan", 1)},
		{name: "empty-expected-with-revision", revision: 1, next: metadataPlan("plan", 1)},
	}
	wrongSession := metadataPlan("plan", 1)
	wrongSession.SessionID = "other-session"
	cases = append(cases, struct {
		name     string
		expected string
		revision int64
		next     *agentplan.Plan
	}{name: "other-session", next: wrongSession})
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ops := newMetadataMongo(t)
			err := NewWithTemplate(ops).SaveSessionPlan(context.Background(), "session", test.expected, test.revision, test.next)
			if !errors.Is(err, portrepository.ErrConflict) || len(ops.updates) != 0 {
				t.Fatalf("invalid plan transition wrote state: err=%v updates=%#v", err, ops.updates)
			}
		})
	}
}

func TestSessionMetadataPropagatesErrorsAndDoesNotAssumeNilResultsSucceeded(t *testing.T) {
	failure := errors.New("fake mongo failure")
	for _, mode := range []string{"error", "nil-result", "empty-result"} {
		t.Run(mode, func(t *testing.T) {
			ops := newMetadataMongo(t)
			ops.override = func(metadataMongoCall) (*gomongo.UpdateResult, error, bool) {
				switch mode {
				case "error":
					return nil, failure, true
				case "nil-result":
					return nil, nil, true
				default:
					return &gomongo.UpdateResult{}, nil, true
				}
			}
			store := NewWithTemplate(ops)
			err := store.TouchSessionMessages(context.Background(), portrepository.SessionRecord{ID: "session"}, 0, false)
			if err == nil || (mode == "error" && !errors.Is(err, failure)) {
				t.Fatalf("initialization result %s accepted: %v", mode, err)
			}
			err = store.SaveSessionPlan(context.Background(), "session", "", 0, metadataPlan("plan", 1))
			if err == nil || (mode == "error" && !errors.Is(err, failure)) {
				t.Fatalf("plan result %s accepted: %v", mode, err)
			}
		})
	}
	ops := newMetadataMongo(t)
	ops.findErr = failure
	err := NewWithTemplate(ops).SaveSessionPlan(context.Background(), "session", "", 0, metadataPlan("plan", 1))
	if !errors.Is(err, failure) {
		t.Fatalf("CAS read failure lost: %v", err)
	}
}

func metadataUsage(tokens int) *portrepository.TokenUsageRecord {
	return &portrepository.TokenUsageRecord{
		PromptTokens: tokens, CompletionTokens: tokens, TotalTokens: tokens * 2,
		ReasoningTokens: tokens / 2, PromptCachedTokens: tokens / 4, Available: true,
	}
}

func metadataPlan(id string, revision int64) *agentplan.Plan {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	return &agentplan.Plan{
		ID: id, SessionID: "session", Revision: revision, Status: agentplan.StatusDraft,
		Goal: "goal", RawContent: "plan", CreatedAt: now, UpdatedAt: now,
		Steps: []agentplan.Step{{ID: "step", Title: "work", Status: agentplan.StepStatusPending}},
	}
}

// This fake interprets the small Mongo query/update subset the helper uses. It
// reports real match/upsert outcomes and rejects conflicting update paths, so a
// broad filter, accidental upsert or dotted write through null cannot pass by
// merely recording that UpdateOne was called.
type metadataMongo struct {
	mongotemplate.Operations
	t              *testing.T
	documents      map[string]bson.M
	updates        []metadataMongoCall
	findErr        error
	messageInserts int
	override       func(metadataMongoCall) (*gomongo.UpdateResult, error, bool)
	afterCommit    func(metadataMongoCall) error
}

type metadataMongoCall struct {
	collection string
	filter     bson.M
	update     bson.M
	upsert     bool
	result     *gomongo.UpdateResult
}

func newMetadataMongo(t *testing.T) *metadataMongo {
	return &metadataMongo{t: t, documents: make(map[string]bson.M)}
}

func (f *metadataMongo) seed(record portrepository.SessionRecord) {
	f.documents[record.ID] = metadataBSON(f.t, mongomapper.SessionDocumentFromRecord(record))
}

func (f *metadataMongo) FindOne(_ context.Context, collection string, filter any, out any, _ ...options.Lister[options.FindOneOptions]) error {
	if f.findErr != nil {
		return f.findErr
	}
	if collection != sessionsCollection {
		f.t.Fatalf("unexpected read collection %s", collection)
	}
	for _, document := range f.documents {
		if metadataMatches(f.t, document, metadataBSON(f.t, filter)) {
			encoded, err := bson.Marshal(document)
			if err != nil {
				return err
			}
			return bson.Unmarshal(encoded, out)
		}
	}
	return mongotemplate.ErrNotFound
}

func (f *metadataMongo) InsertOne(context.Context, string, any) (*gomongo.InsertOneResult, error) {
	f.messageInserts++
	f.t.Fatal("session metadata helper inserted a message")
	return nil, errors.New("unexpected insert")
}

func (f *metadataMongo) UpdateOne(_ context.Context, collection string, filter, update any, opts ...options.Lister[options.UpdateOneOptions]) (*gomongo.UpdateResult, error) {
	f.t.Helper()
	if collection != sessionsCollection {
		f.t.Fatalf("metadata wrote unexpected collection %s", collection)
	}
	settings := options.UpdateOneOptions{}
	for _, option := range opts {
		for _, apply := range option.List() {
			if err := apply(&settings); err != nil {
				return nil, err
			}
		}
	}
	call := metadataMongoCall{collection: collection, filter: metadataBSON(f.t, filter), update: metadataBSON(f.t, update), upsert: settings.Upsert != nil && *settings.Upsert}
	if f.override != nil {
		if result, err, handled := f.override(call); handled {
			call.result = result
			f.updates = append(f.updates, call)
			return result, err
		}
	}
	metadataCheckUpdatePaths(f.t, call.update)
	var document bson.M
	for _, candidate := range f.documents {
		if metadataMatches(f.t, candidate, call.filter) {
			document = candidate
			break
		}
	}
	result := &gomongo.UpdateResult{}
	inserted := false
	if document != nil {
		result.MatchedCount = 1
	} else if call.upsert {
		id, ok := call.filter["_id"].(string)
		if !ok {
			f.t.Fatal("upsert requires an identity")
		}
		if _, exists := f.documents[id]; exists {
			return nil, errors.New("duplicate session identity from conditional upsert")
		}
		document = bson.M{"_id": id}
		f.documents[id] = document
		result.UpsertedCount, result.UpsertedID = 1, id
		inserted = true
	}
	if document != nil {
		before := metadataBSON(f.t, document)
		for operator, raw := range call.update {
			fields := raw.(bson.M)
			for path, value := range fields {
				switch operator {
				case "$setOnInsert":
					if inserted {
						metadataSet(f.t, document, path, value)
					}
				case "$set":
					metadataSet(f.t, document, path, value)
				case "$max":
					current, exists := metadataGet(document, path)
					if !exists || metadataCompare(f.t, current, value) < 0 {
						metadataSet(f.t, document, path, value)
					}
				case "$unset":
					delete(document, path)
				default:
					f.t.Fatalf("unhandled update operator %s", operator)
				}
			}
		}
		if !inserted && !reflect.DeepEqual(before, document) {
			result.ModifiedCount = 1
		}
	}
	call.result = result
	f.updates = append(f.updates, call)
	if document != nil && f.afterCommit != nil {
		if err := f.afterCommit(call); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func metadataBSON(t *testing.T, value any) bson.M {
	t.Helper()
	encoded, err := bson.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	result := bson.M{}
	decoder := bson.NewDecoder(bson.NewDocumentReader(strings.NewReader(string(encoded))))
	decoder.DefaultDocumentM()
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func metadataMatches(t *testing.T, document, filter bson.M) bool {
	t.Helper()
	for path, condition := range filter {
		if path == "$or" || path == "$and" {
			matched := path == "$and"
			for _, clause := range condition.(bson.A) {
				ok := metadataMatches(t, document, clause.(bson.M))
				if path == "$or" {
					matched = matched || ok
				} else {
					matched = matched && ok
				}
			}
			if !matched {
				return false
			}
			continue
		}
		value, exists := metadataGet(document, path)
		if operators, ok := condition.(bson.M); ok {
			for operator, operand := range operators {
				switch operator {
				case "$exists":
					if exists != operand.(bool) {
						return false
					}
				case "$lt", "$lte":
					if !exists || value == nil {
						return false
					}
					comparison := metadataCompare(t, value, operand)
					if comparison > 0 || (operator == "$lt" && comparison == 0) {
						return false
					}
				default:
					t.Fatalf("unhandled filter operator %s", operator)
				}
			}
		} else if condition == nil {
			if exists && value != nil {
				return false
			}
		} else if !exists || !reflect.DeepEqual(value, condition) {
			return false
		}
	}
	return true
}

func metadataGet(document bson.M, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var value any = document
	for _, part := range parts {
		object, ok := value.(bson.M)
		if !ok {
			return nil, false
		}
		value, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

func metadataSet(t *testing.T, document bson.M, path string, value any) {
	t.Helper()
	parts := strings.Split(path, ".")
	for _, part := range parts[:len(parts)-1] {
		next, exists := document[part]
		if !exists {
			next = bson.M{}
			document[part] = next
		}
		object, ok := next.(bson.M)
		if !ok {
			t.Fatalf("cannot write %s through non-object %s=%#v", path, part, next)
		}
		document = object
	}
	document[parts[len(parts)-1]] = value
}

func metadataCompare(t *testing.T, left, right any) int {
	t.Helper()
	if left == nil {
		if right == nil {
			return 0
		}
		return -1
	}
	number := func(value any) int64 {
		switch typed := value.(type) {
		case int32:
			return int64(typed)
		case int64:
			return typed
		case bson.DateTime:
			return int64(typed)
		case bool:
			if typed {
				return 1
			}
			return 0
		default:
			t.Fatalf("unhandled comparison type %T", value)
			return 0
		}
	}
	a, b := number(left), number(right)
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func metadataCheckUpdatePaths(t *testing.T, update bson.M) {
	t.Helper()
	var paths []string
	for _, raw := range update {
		for path := range raw.(bson.M) {
			for _, previous := range paths {
				if path == previous || strings.HasPrefix(path, previous+".") || strings.HasPrefix(previous, path+".") {
					t.Fatalf("Mongo update contains conflicting paths %s and %s", previous, path)
				}
			}
			paths = append(paths, path)
		}
	}
}
