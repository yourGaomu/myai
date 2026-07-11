package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"myai/core/adapter/persistence/mongo/authorization/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainauthorization "myai/core/domain/authorization"
	authorizationport "myai/core/port/authorization"
)

func TestStoreImplementsAuthorizationPort(t *testing.T) {
	var _ authorizationport.Store = (*Store)(nil)
}

func TestStoreGetsAuthorizationThroughOperations(t *testing.T) {
	now := time.Date(2026, 7, 11, 13, 0, 0, 0, time.UTC)
	operations := &fakeOperations{document: po.Document{
		ID: "auth-1", UserID: "user-1", DeviceID: "device-1", CreatedAt: now,
	}}
	store := NewWithOperations(operations)

	authorization, err := store.Get(context.Background(), "auth-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if operations.collection != collection || authorization.ID != "auth-1" || authorization.UserID != "user-1" {
		t.Fatalf("unexpected delegation: collection=%q authorization=%#v", operations.collection, authorization)
	}
}

func TestStoreTranslatesMongoNotFound(t *testing.T) {
	store := NewWithOperations(&fakeOperations{findOneErr: mongotemplate.ErrNotFound})
	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, authorizationport.ErrNotFound) {
		t.Fatalf("Get() error = %v, want authorization not found", err)
	}
}

func TestStoreListsMappedAuthorizations(t *testing.T) {
	operations := &fakeOperations{documents: []po.Document{{ID: "auth-1"}, {ID: "auth-2"}}}
	store := NewWithOperations(operations)
	store.now = func() time.Time { return time.Date(2026, 7, 11, 13, 0, 0, 0, time.UTC) }

	authorizations, err := store.ListActive(context.Background(), "user-1", "device-1")
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(authorizations) != 2 || authorizations[0].ID != "auth-1" || authorizations[1].ID != "auth-2" {
		t.Fatalf("unexpected authorizations: %#v", authorizations)
	}
}

func TestStoreSaveAndTouchUseOperations(t *testing.T) {
	operations := &fakeOperations{updateResult: &gomongo.UpdateResult{MatchedCount: 1}}
	store := NewWithOperations(operations)
	authorization := domainauthorization.ClientAuthorization{ID: "auth-1", UserID: "user-1"}

	if err := store.Save(context.Background(), authorization); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if operations.collection != collection || operations.update == nil {
		t.Fatalf("unexpected save delegation: %#v", operations)
	}
	if err := store.Touch(context.Background(), authorization.ID, time.Now()); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}
}

type fakeOperations struct {
	collection   string
	document     po.Document
	documents    []po.Document
	findOneErr   error
	update       any
	updateResult *gomongo.UpdateResult
}

func (f *fakeOperations) FindOne(_ context.Context, collection string, _ any, out any, _ ...options.Lister[options.FindOneOptions]) error {
	f.collection = collection
	if f.findOneErr != nil {
		return f.findOneErr
	}
	if target, ok := out.(*po.Document); ok {
		*target = f.document
	}
	return nil
}

func (f *fakeOperations) FindAll(_ context.Context, collection string, _ any, out any, _ ...options.Lister[options.FindOptions]) error {
	f.collection = collection
	if target, ok := out.(*[]po.Document); ok {
		*target = append([]po.Document(nil), f.documents...)
	}
	return nil
}

func (f *fakeOperations) UpdateOne(_ context.Context, collection string, _ any, update any, _ ...options.Lister[options.UpdateOneOptions]) (*gomongo.UpdateResult, error) {
	f.collection = collection
	f.update = update
	if f.updateResult != nil {
		return f.updateResult, nil
	}
	return &gomongo.UpdateResult{MatchedCount: 1}, nil
}

func (*fakeOperations) InsertOne(context.Context, string, any) (*gomongo.InsertOneResult, error) {
	return &gomongo.InsertOneResult{}, nil
}

func (*fakeOperations) DeleteMany(context.Context, string, any) (*gomongo.DeleteResult, error) {
	return &gomongo.DeleteResult{}, nil
}

func (*fakeOperations) Count(context.Context, string, any) (int64, error) {
	return 0, nil
}
