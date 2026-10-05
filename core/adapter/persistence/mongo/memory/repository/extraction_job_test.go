package repository

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
	"testing"
	"time"
)

type jobOperations struct {
	mongotemplate.Operations
	filter  any
	update  any
	matched int64
	upsert  bool
}

func (o *jobOperations) UpdateOne(_ context.Context, _ string, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*gomongo.UpdateResult, error) {
	o.filter, o.update = filter, update
	var settings options.UpdateOneOptions
	for _, opt := range opts {
		for _, apply := range opt.List() {
			if err := apply(&settings); err != nil {
				return nil, err
			}
		}
	}
	o.upsert = settings.Upsert != nil && *settings.Upsert
	return &gomongo.UpdateResult{MatchedCount: o.matched}, nil
}

func TestExtractionCASRejectsStaleVersionWithoutUpsert(t *testing.T) {
	operations := &jobOperations{}
	repository := &Repository{template: operations}
	now := time.Now()
	job := domainmemory.ExtractionJob{ID: "job", AgentRunID: "run", ExtractorVersion: "v1", Status: domainmemory.JobRunning, CreatedAt: now, UpdatedAt: now, Revision: 8}
	if err := repository.CompareAndSwapExtractionJob(context.Background(), 7, job); !errors.Is(err, memoryport.ErrConflict) {
		t.Fatalf("stale owner was accepted: %v", err)
	}
	filter := operations.filter.(bson.M)
	if filter["revision"] != 7 || filter["_id"] != "job" || operations.upsert {
		t.Fatalf("unsafe CAS: %#v upsert=%v", filter, operations.upsert)
	}
	operations.matched = 1
	if err := repository.CompareAndSwapExtractionJob(context.Background(), 7, job); err != nil {
		t.Fatal(err)
	}
	fields := operations.update.(bson.M)["$set"].(bson.M)
	if _, exists := fields["_id"]; exists {
		t.Fatal("CAS attempts to update immutable id")
	}
	if fields["lease_until"] != nil || fields["last_error"] != "" {
		t.Fatalf("stale lease or error not cleared: %#v", fields)
	}
}
