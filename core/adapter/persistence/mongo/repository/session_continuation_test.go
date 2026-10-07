package repository

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"myai/core/adapter/persistence/mongo/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	repository "myai/core/port/repository"
)

type controlMongoOperations struct {
	fakeMongoOperations
	paused  bool
	exists  bool
	deleted bool
}

func (f *controlMongoOperations) FindOne(ctx context.Context, collection string, filter any, out any, opts ...options.Lister[options.FindOneOptions]) error {
	if collection == continuationControlsCollection {
		if !f.exists {
			return mongotemplate.ErrNotFound
		}
		out.(*po.ContinuationControl).Paused = f.paused
		return nil
	}
	if f.deleted {
		return mongotemplate.ErrNotFound
	}
	return f.fakeMongoOperations.FindOne(ctx, collection, filter, out, opts...)
}
func (f *controlMongoOperations) UpdateOne(ctx context.Context, collection string, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*gomongo.UpdateResult, error) {
	if collection == continuationControlsCollection {
		f.paused = update.(bson.M)["$set"].(bson.M)["paused"].(bool)
		f.exists = true
	}
	return f.fakeMongoOperations.UpdateOne(ctx, collection, filter, update, opts...)
}

func TestContinuationPauseSurvivesSessionSnapshotsAndStoreRecreation(t *testing.T) {
	ops := &controlMongoOperations{fakeMongoOperations: fakeMongoOperations{session: po.SessionDocument{ID: "parent"}}}
	store := NewWithTemplate(ops)
	ctx := context.Background()
	allowed, err := store.ContinuationAllowed(ctx, "parent")
	if err != nil || !allowed {
		t.Fatalf("default eligibility=%v %v", allowed, err)
	}
	if err := store.SetContinuationPaused(ctx, "parent", true); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSession(ctx, repository.SessionRecord{ID: "parent"}); err != nil {
		t.Fatal(err)
	}
	restarted := NewWithTemplate(ops)
	allowed, err = restarted.ContinuationAllowed(ctx, "parent")
	if err != nil || allowed {
		t.Fatalf("snapshot cleared pause: %v %v", allowed, err)
	}
	if err := restarted.SetContinuationPaused(ctx, "parent", false); err != nil {
		t.Fatal(err)
	}
	allowed, err = restarted.ContinuationAllowed(ctx, "parent")
	if err != nil || !allowed {
		t.Fatalf("resume=%v %v", allowed, err)
	}
	ops.deleted = true
	allowed, err = restarted.ContinuationAllowed(ctx, "parent")
	if err != nil || allowed {
		t.Fatalf("deleted session resumed: %v %v", allowed, err)
	}
}
