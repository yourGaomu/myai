package repository

import (
	"context"
	"reflect"
	"regexp"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
)

type memoryOperations struct {
	mongotemplate.Operations
	collection string
	filter     any
	update     any
	find       options.FindOptions
}

func (operations *memoryOperations) FindAll(_ context.Context, collection string, filter any, _ any, opts ...options.Lister[options.FindOptions]) error {
	operations.collection, operations.filter = collection, filter
	operations.find = options.FindOptions{}
	for _, option := range opts {
		for _, apply := range option.List() {
			if err := apply(&operations.find); err != nil {
				return err
			}
		}
	}
	return nil
}

func (operations *memoryOperations) UpdateOne(_ context.Context, collection string, filter any, update any, _ ...options.Lister[options.UpdateOneOptions]) (*gomongo.UpdateResult, error) {
	operations.collection, operations.filter, operations.update = collection, filter, update
	return &gomongo.UpdateResult{MatchedCount: 1}, nil
}

func TestListHonorsLimitsAndClampsAtMaximum(t *testing.T) {
	for _, test := range []struct {
		name  string
		limit int
		want  int64
	}{
		{name: "default", limit: 0, want: 50},
		{name: "negative", limit: -1, want: 50},
		{name: "one", limit: 1, want: 1},
		{name: "catalog size", limit: 100, want: 100},
		{name: "above old ceiling", limit: 101, want: 101},
		{name: "recall pool", limit: 200, want: 200},
		{name: "above ceiling", limit: 201, want: 200},
		{name: "large request", limit: 10000, want: 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			operations := &memoryOperations{}
			repository := &Repository{template: operations}
			_, err := repository.List(context.Background(), memoryport.ListFilter{Statuses: []domainmemory.Status{domainmemory.StatusActive}, Limit: test.limit})
			if err != nil {
				t.Fatal(err)
			}
			if operations.find.Limit == nil || *operations.find.Limit != test.want {
				t.Fatalf("requested %d memories: Mongo limit=%v, want %d", test.limit, operations.find.Limit, test.want)
			}
			if operations.collection != "ai_memories" || !reflect.DeepEqual(operations.filter.(bson.M)["status"], bson.M{"$in": []string{"active"}}) {
				t.Fatalf("wrong collection or status filter: %s %#v", operations.collection, operations.filter)
			}
		})
	}
}

func TestRecallQueryFiltersCurrentContentBeforeLimitAndSortsByRevisionTime(t *testing.T) {
	operations := &memoryOperations{}
	repository := &Repository{template: operations}
	_, err := repository.List(context.Background(), memoryport.ListFilter{
		Statuses: []domainmemory.Status{domainmemory.StatusActive}, Limit: 200,
		SearchTerms: []string{"redis", "数据库"}, OrderByContent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if operations.find.Limit == nil || *operations.find.Limit != 200 {
		t.Fatalf("recall pool was reduced: %#v", operations.find)
	}
	wantSort := bson.D{{Key: "revisions.created_at", Value: -1}, {Key: "_id", Value: 1}}
	if !reflect.DeepEqual(operations.find.Sort, wantSort) {
		t.Fatalf("recall must sort by content history, with a stable tie break: %#v", operations.find.Sort)
	}
	query := operations.filter.(bson.M)
	alternatives, ok := query["$or"].(bson.A)
	if !ok || len(alternatives) != 3 {
		t.Fatalf("Mongo must filter titles, tags and current content before the limit: %#v", query)
	}
	regex := alternatives[0].(bson.M)["title"].(bson.M)
	if !reflect.DeepEqual(alternatives[1].(bson.M)["tags"], regex) || regex["$options"] != "i" {
		t.Fatalf("title and tag matching use different quality gates: %#v", alternatives)
	}
	pattern := regexp.MustCompile("(?i)" + regex["$regex"].(string))
	for _, test := range []struct {
		text string
		want bool
	}{
		{text: "Restart REDIS safely", want: true},
		{text: "数据库连接恢复", want: true},
		{text: "Redis连接池", want: true},
		{text: "Predictable layout", want: false},
		{text: "rediscover palette", want: false},
	} {
		if got := pattern.MatchString(test.text); got != test.want {
			t.Fatalf("Mongo regex against %q matched=%v, want %v", test.text, got, test.want)
		}
	}
	// Check the safety-relevant part of the expression: a retired revision
	// cannot match just because it contains a formerly relevant phrase.
	expression := alternatives[2].(bson.M)["$expr"].(bson.M)
	mapping := expression["$anyElementTrue"].(bson.A)[0].(bson.M)["$map"].(bson.M)
	conditions := mapping["in"].(bson.M)["$and"].(bson.A)
	if !reflect.DeepEqual(conditions[0], bson.M{"$eq": bson.A{"$$revision.version", "$current_version"}}) {
		t.Fatalf("Mongo search can admit retired content: %#v", mapping)
	}
	contentRegex := conditions[1].(bson.M)["$regexMatch"].(bson.M)
	if contentRegex["regex"] != regex["$regex"] || contentRegex["options"] != "i" {
		t.Fatalf("content search bypasses the title/tag quality gate: %#v", contentRegex)
	}
}

func TestRecordUseDoesNotWriteContentOrUpdatedAt(t *testing.T) {
	operations := &memoryOperations{}
	repository := &Repository{template: operations}
	usedAt := time.Date(2026, 11, 10, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	if err := repository.RecordUse(context.Background(), " memory-1 ", usedAt); err != nil {
		t.Fatal(err)
	}
	if operations.collection != "ai_memories" || !reflect.DeepEqual(operations.filter, bson.M{"_id": "memory-1", "status": "active"}) {
		t.Fatalf("usage update must target only the active memory: %s %#v", operations.collection, operations.filter)
	}
	want := bson.M{"$inc": bson.M{"use_count": 1}, "$set": bson.M{"last_used_at": usedAt.UTC()}}
	if !reflect.DeepEqual(operations.update, want) {
		t.Fatalf("usage update must leave all content fields, including updated_at, untouched: %#v", operations.update)
	}
}
