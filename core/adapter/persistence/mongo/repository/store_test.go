package repository

import (
	"context"
	"testing"

	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"myai/core/adapter/persistence/mongo/po"

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

type fakeMongoOperations struct {
	collection string
	document   any
	session    po.SessionDocument
	sessions   []po.SessionDocument
}

func (f *fakeMongoOperations) FindOne(_ context.Context, collection string, _ any, out any, _ ...options.Lister[options.FindOneOptions]) error {
	f.collection = collection
	switch target := out.(type) {
	case *po.SessionDocument:
		*target = f.session
	}
	return nil
}

func (f *fakeMongoOperations) FindAll(_ context.Context, collection string, _ any, out any, _ ...options.Lister[options.FindOptions]) error {
	f.collection = collection
	switch target := out.(type) {
	case *[]po.SessionDocument:
		*target = append([]po.SessionDocument(nil), f.sessions...)
	}
	return nil
}

func (f *fakeMongoOperations) UpdateOne(context.Context, string, any, any, ...options.Lister[options.UpdateOneOptions]) (*gomongo.UpdateResult, error) {
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
