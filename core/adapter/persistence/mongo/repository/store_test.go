package repository

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"myai/core/adapter/persistence/mongo/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"

	repository "myai/core/port/repository"
)

func TestStoreDelegatesSessionReadToTemplate(t *testing.T) {
	operations := &fakeMongoOperations{
		session: po.SessionDocument{ID: "session-1", Model: "gpt-5"},
	}
	store := NewWithTemplate(operations)

	record, err := store.GetSession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if operations.collection != sessionsCollection || record.ID != "session-1" {
		t.Fatalf("unexpected delegation: operations=%#v record=%#v", operations, record)
	}
}

func TestStoreDelegatesMessageInsertToTemplate(t *testing.T) {
	operations := &fakeMongoOperations{}
	store := NewWithTemplate(operations)
	message := repository.MessageRecord{ID: "message-1", SessionID: "session-1"}

	if err := store.SaveMessage(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if operations.collection != messagesCollection {
		t.Fatalf("expected messages collection, got %q", operations.collection)
	}
	inserted, ok := operations.document.(po.MessageDocument)
	if !ok || inserted.ID != "message-1" {
		t.Fatalf("unexpected inserted document: %#v", operations.document)
	}
}

func TestStoreDelegatesSessionListToTemplate(t *testing.T) {
	operations := &fakeMongoOperations{
		sessions: []po.SessionDocument{{ID: "session-1"}},
	}
	store := NewWithTemplate(operations)

	records, err := store.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if operations.collection != sessionsCollection || len(records) != 1 || records[0].ID != "session-1" {
		t.Fatalf("unexpected session list: operations=%#v records=%#v", operations, records)
	}
}

func TestStoreListsMessagesAfterCursorInMongo(t *testing.T) {
	createdAt := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	operations := &fakeMongoOperations{
		message:  po.MessageDocument{ID: "message-1", SessionID: "session-1", Sequence: 41, CreatedAt: createdAt},
		messages: []po.MessageDocument{{ID: "message-2", SessionID: "session-1", Sequence: 42, CreatedAt: createdAt.Add(time.Nanosecond)}},
	}
	store := NewWithTemplate(operations)

	records, fullSync, err := store.ListMessagesAfter(context.Background(), "session-1", "message-1", 25)
	if err != nil || fullSync || len(records) != 1 || records[0].ID != "message-2" {
		t.Fatalf("unexpected incremental result: records=%#v fullSync=%v err=%v", records, fullSync, err)
	}
	filter, ok := operations.findAllFilter.(bson.M)
	if !ok || filter["$and"] == nil {
		t.Fatalf("expected tuple cursor filter, got %#v", operations.findAllFilter)
	}
	if operations.findOptions.Limit == nil || *operations.findOptions.Limit != 25 || operations.findOptions.Sort == nil {
		t.Fatalf("expected database sort and limit, got %#v", operations.findOptions)
	}
}

func TestStoreRequiresFullSyncWhenMessageCursorIsMissing(t *testing.T) {
	operations := &fakeMongoOperations{findOneErr: mongotemplate.ErrNotFound}
	store := NewWithTemplate(operations)

	records, fullSync, err := store.ListMessagesAfter(context.Background(), "session-1", "missing", 25)
	if err != nil || !fullSync || len(records) != 0 || operations.findAllCalls != 0 {
		t.Fatalf("unexpected missing cursor result: records=%#v fullSync=%v err=%v operations=%#v", records, fullSync, err, operations)
	}
}

type fakeMongoOperations struct {
	collection    string
	document      any
	session       po.SessionDocument
	sessions      []po.SessionDocument
	message       po.MessageDocument
	messages      []po.MessageDocument
	findOneErr    error
	findAllFilter any
	findAllCalls  int
	findOptions   options.FindOptions
}

func (f *fakeMongoOperations) FindOne(_ context.Context, collection string, _ any, out any, _ ...options.Lister[options.FindOneOptions]) error {
	f.collection = collection
	if f.findOneErr != nil {
		return f.findOneErr
	}
	switch target := out.(type) {
	case *po.SessionDocument:
		*target = f.session
	case *po.MessageDocument:
		*target = f.message
	}
	return nil
}

func (f *fakeMongoOperations) FindAll(_ context.Context, collection string, filter any, out any, opts ...options.Lister[options.FindOptions]) error {
	f.collection = collection
	f.findAllCalls++
	f.findAllFilter = filter
	f.findOptions = options.FindOptions{}
	for _, option := range opts {
		for _, apply := range option.List() {
			if err := apply(&f.findOptions); err != nil {
				return err
			}
		}
	}
	switch target := out.(type) {
	case *[]po.SessionDocument:
		*target = append([]po.SessionDocument(nil), f.sessions...)
	case *[]po.MessageDocument:
		*target = append([]po.MessageDocument(nil), f.messages...)
	}
	return nil
}

func (f *fakeMongoOperations) UpdateOne(context.Context, string, any, any, ...options.Lister[options.UpdateOneOptions]) (*gomongo.UpdateResult, error) {
	return &gomongo.UpdateResult{}, nil
}

func (f *fakeMongoOperations) UpdateMany(context.Context, string, any, any, ...options.Lister[options.UpdateManyOptions]) (*gomongo.UpdateResult, error) {
	return &gomongo.UpdateResult{}, nil
}

func (f *fakeMongoOperations) InsertOne(_ context.Context, collection string, document any) (*gomongo.InsertOneResult, error) {
	f.collection = collection
	f.document = document
	return &gomongo.InsertOneResult{}, nil
}

func (f *fakeMongoOperations) DeleteMany(context.Context, string, any) (*gomongo.DeleteResult, error) {
	return &gomongo.DeleteResult{}, nil
}

func (f *fakeMongoOperations) Count(context.Context, string, any) (int64, error) {
	return 0, nil
}
